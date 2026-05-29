package client

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitcode.com/sheng_wang/cf-tool_modify/util"
	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
)

// safeName converts a blog title into a safe filename.
func safeName(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	reg := regexp.MustCompile(`[^\w]+`)
	title = strings.Trim(reg.ReplaceAllString(title, "_"), "_")
	if title == "" {
		title = "material"
	}
	return title
}

// extractCSRF extracts the X-Csrf-Token from a page body.
func extractCSRF(body []byte) string {
	reg := regexp.MustCompile(`X-Csrf-Token" content="([^"]+)"`)
	if m := reg.FindSubmatch(body); m != nil {
		return string(m[1])
	}
	return ""
}

// fetchProblemTutorial fetches the editorial text for a single problem via the
// Codeforces AJAX endpoint, returning rendered HTML.
func (c *Client) fetchProblemTutorial(problemCode, csrf, referer string) string {
	form := url.Values{"problemCode": {problemCode}}
	req, err := http.NewRequest("POST", "https://codeforces.com/data/problemTutorial",
		strings.NewReader(form.Encode()))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Csrf-Token", csrf)
	req.Header.Set("Referer", referer)

	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	var result struct {
		Success string `json:"success"`
		HTML    string `json:"html"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Success != "true" {
		return ""
	}
	return result.HTML
}

// renderSpoiler renders a .spoiler div: expands the content, fetching tutorial
// text for Editorial spoilers.
func (c *Client) renderSpoiler(s *goquery.Selection, csrf, blogURL string) string {
	titleText := strings.TrimSpace(s.Find("b.spoiler-title").Text())
	content := s.Find(".spoiler-content")

	var sb strings.Builder
	sb.WriteString("### ")
	sb.WriteString(titleText)
	sb.WriteString("\n\n")

	// Check for lazy-loaded problem tutorial
	tutDiv := content.Find(".problemTutorial")
	if tutDiv.Length() > 0 {
		problemCode, _ := tutDiv.Attr("problemcode")
		if problemCode != "" && csrf != "" {
			html := c.fetchProblemTutorial(problemCode, csrf, blogURL)
			if html != "" {
				doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
				if err == nil {
					sb.WriteString(nodeText(doc.Find(".ttypography").First()))
					sb.WriteString("\n")
					return sb.String()
				}
			}
		}
		sb.WriteString("*(Tutorial not available)*\n")
		return sb.String()
	}

	// Code block inside spoiler
	pre := content.Find("pre code")
	if pre.Length() > 0 {
		sb.WriteString("```\n")
		sb.WriteString(html.UnescapeString(pre.Text()))
		sb.WriteString("\n```\n")
		return sb.String()
	}

	// Generic spoiler content
	sb.WriteString(nodeText(content))
	sb.WriteString("\n")
	return sb.String()
}

// fetchBlogToMD fetches a Codeforces blog entry and renders it as Markdown,
// expanding all spoiler blocks and fetching lazy-loaded editorial text.
func (c *Client) fetchBlogToMD(blogURL string) (string, error) {
	body, err := util.GetBody(c.client, blogURL)
	if err != nil {
		return "", err
	}
	csrf := extractCSRF(body)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("cannot parse HTML: %v", err)
	}

	var sb strings.Builder

	title := strings.TrimSpace(doc.Find(".title").First().Text())
	if title != "" {
		sb.WriteString("# ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}

	doc.Find(".ttypography").First().Children().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "div" {
			if s.HasClass("spoiler") {
				sb.WriteString(c.renderSpoiler(s, csrf, blogURL))
				return
			}
		}
		sb.WriteString(nodeText(s))
		sb.WriteByte('\n')
	})

	content := sb.String()
	collapse := regexp.MustCompile(`\n{3,}`)
	content = collapse.ReplaceAllString(content, "\n\n")
	content = strings.ReplaceAll(content, "$$$", "$")
	return content, nil
}

// FetchTutorial fetches all Contest materials blog entries for a contest and
// saves each as <title>.md under the contest path.
func (c *Client) FetchTutorial(info Info, host string) error {
	if info.ContestID == "" {
		return fmt.Errorf("you have to specify the Contest ID")
	}

	contestURL := fmt.Sprintf(host+"/contest/%v", info.ContestID)
	body, err := util.GetBody(c.client, contestURL)
	if err != nil {
		return err
	}

	s := string(body)
	start := strings.Index(s, "Contest materials")
	if start < 0 {
		return fmt.Errorf("cannot find Contest materials for contest %v", info.ContestID)
	}
	block := s[start:]
	ulStart := strings.Index(block, "<ul>")
	ulEnd := strings.Index(block, "</ul>")
	if ulStart < 0 || ulEnd < 0 {
		return fmt.Errorf("cannot parse Contest materials block for contest %v", info.ContestID)
	}
	block = block[ulStart : ulEnd+5]

	reg := regexp.MustCompile(`href="(/blog/entry/\d+)"[^>]*>([^<]+)`)
	matches := reg.FindAllSubmatch([]byte(block), -1)
	if len(matches) == 0 {
		return fmt.Errorf("no blog entries found in Contest materials for contest %v", info.ContestID)
	}

	savePath := info.Path()
	if err := os.MkdirAll(savePath, os.ModePerm); err != nil {
		return err
	}

	for _, m := range matches {
		blogURL := host + string(m[1])
		linkText := strings.TrimSpace(string(m[2]))
		color.Cyan("Fetching %v (%v)", linkText, blogURL)

		content, err := c.fetchBlogToMD(blogURL)
		if err != nil {
			color.Red("Failed %v: %v", linkText, err)
			continue
		}

		filename := safeName(linkText) + ".md"
		outFile := filepath.Join(savePath, filename)
		if err := ioutil.WriteFile(outFile, []byte(content), 0644); err != nil {
			color.Red("Cannot write %v: %v", outFile, err)
			continue
		}
		color.Green("Saved %v", outFile)
	}
	return nil
}
