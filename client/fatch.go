package client

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"gitcode.com/sheng_wang/cf-tool_modify/util"
	"github.com/PuerkitoBio/goquery"
)

func nodeText(sel *goquery.Selection, visited map[*html.Node]bool) string {
	var sb strings.Builder
	sel.Contents().Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		// 对于 div 元素，检查是否已被处理过，避免重复解析
		if goquery.NodeName(s) == "div" {
			if visited[node] {
				return
			}
			visited[node] = true
		}
		switch goquery.NodeName(s) {
		case "br":
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "#text":
			sb.WriteString(s.Text())
		case "p", "div":
			sb.WriteString(nodeText(s, visited))
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "ul", "ol":
			// 嵌套列表在 <li> 内部时，先加换行使子项另起一行
			if goquery.NodeName(sel) == "li" {
				sb.WriteByte('\n')
				sb.WriteByte('\n')
			}
			s.ChildrenFiltered("li").Each(func(_ int, li *goquery.Selection) {
				sb.WriteString("- ")
				sb.WriteString(strings.TrimSpace(nodeText(li, visited)))
				sb.WriteByte('\n')
				sb.WriteByte('\n')
			})
		default:
			sb.WriteString(nodeText(s, visited))
		}
	})
	return sb.String()
}

func sectionText(sel *goquery.Selection, visited map[*html.Node]bool) string {
	var sb strings.Builder
	// title
	title := strings.TrimSpace(sel.Find(".section-title").Text())
	if title != "" {
		sb.WriteString("## ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}
	sel.Find(".section-title").Remove()
	text := strings.TrimSpace(nodeText(sel, visited))
	// collapse multiple blank lines
	reg := regexp.MustCompile(`\n{3,}`)
	text = reg.ReplaceAllString(text, "\n\n")
	sb.WriteString(text)
	sb.WriteString("\n\n")
	return sb.String()
}

// renderSpoiler renders a .spoiler div: expands the content, fetching tutorial
// text for Editorial spoilers.
func (c *Client) renderSpoiler(s *goquery.Selection, csrf, blogURL string, visited map[*html.Node]bool) string {
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
					sb.WriteString(nodeText(doc.Find(".ttypography").First(), visited))
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
		sb.WriteString(stdhtml.UnescapeString(pre.Text()))
		sb.WriteString("\n```\n")
		return sb.String()
	}

	// Generic spoiler content
	sb.WriteString(nodeText(content, visited))
	sb.WriteString("\n")
	return sb.String()
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

// FetchStatement fetches the codeforce content to md file
func (c *Client) FetchUrlContentToMdFile(URL, path string, fileName string, mu *sync.Mutex) error {
	body, err := util.GetBody(c.client, URL)
	if err != nil {
		return err
	}
	csrf := extractCSRF(body)

	_, err = findHandle(body)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("cannot parse HTML: %v", err)
	}

	var sb strings.Builder

	// problem title
	title := strings.TrimSpace(doc.Find(".title").First().Text())
	if title != "" {
		sb.WriteString("# ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}

	// time / memory limits
	limits := strings.TrimSpace(doc.Find(".time-limit").Text())
	if limits != "" {
		sb.WriteString("**")
		sb.WriteString(strings.TrimSpace(doc.Find(".property-title").First().Text()))
		sb.WriteString("** ")
		doc.Find(".time-limit .property-title").Remove()
		sb.WriteString(strings.TrimSpace(doc.Find(".time-limit").Text()))
		sb.WriteString("\n")
		_ = limits
	}
	sb.WriteString("\n")
	memLimit := strings.TrimSpace(doc.Find(".memory-limit").Text())
	if memLimit != "" {
		sb.WriteString("**")
		sb.WriteString(strings.TrimSpace(doc.Find(".memory-limit .property-title").Text()))
		sb.WriteString("** ")
		doc.Find(".memory-limit .property-title").Remove()
		sb.WriteString(strings.TrimSpace(doc.Find(".memory-limit").Text()))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// main statement sections (problem-statement children divs)
	visited := make(map[*html.Node]bool)
	doc.Find(".problem-statement > div").Each(func(_ int, s *goquery.Selection) {
		// skip header div (title/limits already handled)
		if s.HasClass("header") {
			return
		}
		// sample test blocks — render as plain text tables
		if s.HasClass("sample-tests") {
			sb.WriteString("## Sample Tests\n\n")
			s.Find(".sample-test").Each(func(i int, st *goquery.Selection) {
				sb.WriteString(fmt.Sprintf("### Sample %d\n\n", i+1))
				inputText := strings.TrimSpace(nodeText(st.Find(".input pre"), visited))
				reg := regexp.MustCompile(`\n{2,}`)
				inputText = reg.ReplaceAllString(inputText, "\n")
				outputText := strings.TrimSpace(nodeText(st.Find(".output pre"), visited))
				outputText = reg.ReplaceAllString(outputText, "\n")
				sb.WriteString("**Input**\n```\n")
				sb.WriteString(inputText)
				sb.WriteString("\n```\n\n**Output**\n```\n")
				sb.WriteString(outputText)
				sb.WriteString("\n```\n\n")
			})
			return
		}
		// note / other sections
		sb.WriteString(sectionText(s, visited))
	})

	doc.Find(".ttypography").First().Children().Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "div" {
			if s.HasClass("problem-statement") {
				return
			}
			if s.HasClass("spoiler") {
				sb.WriteString(c.renderSpoiler(s, csrf, URL, visited))
				return
			}
		}
		sb.WriteString(nodeText(s, visited))
		sb.WriteString("\n\n")
	})

	content := sb.String()
	// trim excessive blank lines globally
	reg := regexp.MustCompile(`\n{3,}`)
	content = reg.ReplaceAllString(content, "\n\n")
	// Codeforces uses $$$ for both inline and display math; replace with $
	content = strings.ReplaceAll(content, "$$$", "$")

	reg = regexp.MustCompile(`(\${1,2})([^\$\n ])`)
	content = reg.ReplaceAllString(content, "$1 $2")
	reg = regexp.MustCompile(`([^\$\n ])(\${1,2})`)
	content = reg.ReplaceAllString(content, "$1 $2")

	outFile := filepath.Join(path, fileName)
	if err := ioutil.WriteFile(outFile, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}
