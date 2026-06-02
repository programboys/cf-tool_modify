package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gitcode.com/sheng_wang/cf-tool_modify/util"
	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
)

func nodeText(sel *goquery.Selection) string {
	var sb strings.Builder
	sel.Contents().Each(func(_ int, s *goquery.Selection) {
		switch goquery.NodeName(s) {
		case "br":
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "#text":
			sb.WriteString(s.Text())
		case "p", "div":
			sb.WriteString(nodeText(s))
			sb.WriteByte('\n')
			sb.WriteByte('\n')
		case "ul", "ol":
			s.Find("li").Each(func(_ int, li *goquery.Selection) {
				sb.WriteString("- ")
				sb.WriteString(strings.TrimSpace(nodeText(li)))
				sb.WriteByte('\n')
				sb.WriteByte('\n')
			})
		default:
			sb.WriteString(nodeText(s))
		}
	})
	return sb.String()
}

func sectionText(sel *goquery.Selection) string {
	var sb strings.Builder
	// title
	title := strings.TrimSpace(sel.Find(".section-title").Text())
	if title != "" {
		sb.WriteString("## ")
		sb.WriteString(title)
		sb.WriteString("\n\n")
	}
	sel.Find(".section-title").Remove()
	text := strings.TrimSpace(nodeText(sel))
	// collapse multiple blank lines
	reg := regexp.MustCompile(`\n{3,}`)
	text = reg.ReplaceAllString(text, "\n\n")
	sb.WriteString(text)
	sb.WriteString("\n\n")
	return sb.String()
}

// FetchStatement fetches the problem statement and saves it as statement.md in path.
func (c *Client) FetchStatement(URL, path string, mu *sync.Mutex) error {
	body, err := util.GetBody(c.client, URL)
	if err != nil {
		return err
	}

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
				inputText := strings.TrimSpace(nodeText(st.Find(".input pre")))
				reg := regexp.MustCompile(`\n{2,}`)
				inputText = reg.ReplaceAllString(inputText, "\n")
				outputText := strings.TrimSpace(nodeText(st.Find(".output pre")))
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
		sb.WriteString(sectionText(s))
	})

	content := sb.String()
	// trim excessive blank lines globally
	reg := regexp.MustCompile(`\n{3,}`)
	content = reg.ReplaceAllString(content, "\n\n")
	// Codeforces uses $$$ for both inline and display math; replace with $
	content = strings.ReplaceAll(content, "$$$", "$")
	content = strings.ReplaceAll(content, "$$", "$")

	outFile := filepath.Join(path, "statement.md")
	if err := ioutil.WriteFile(outFile, []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

// Statement fetches statements for the problems described by info.
func (c *Client) Statement(info Info) (err error) {
	color.Cyan("Fetch statement " + info.Hint())

	problemID := info.ProblemID
	info.ProblemID = "%v"
	urlFormatter, err := info.ProblemURL(c.host)
	if err != nil {
		return
	}
	info.ProblemID = ""

	var problems []string
	if problemID == "" {
		statics, err := c.Statis(info)
		if err != nil {
			return err
		}
		for _, p := range statics {
			problems = append(problems, p.ID)
		}
	} else {
		problems = []string{problemID}
	}

	contestPath := info.Path()

	wg := sync.WaitGroup{}
	wg.Add(len(problems))
	mu := sync.Mutex{}
	for _, pid := range problems {
		ppath := filepath.Join(contestPath, strings.ToLower(pid))
		go func(pid, ppath string) {
			defer wg.Done()
			if e := os.MkdirAll(ppath, os.ModePerm); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", pid, e.Error())
				mu.Unlock()
				return
			}
			URL := fmt.Sprintf(urlFormatter, pid)
			e := c.FetchStatement(URL, ppath, &mu)
			mu.Lock()
			if e != nil {
				color.Red("Failed %v: %v", pid, e.Error())
			} else {
				color.Green("Fetched statement for %v -> %v/statement.md", pid, ppath)
			}
			mu.Unlock()
		}(pid, ppath)
	}
	wg.Wait()
	return
}
