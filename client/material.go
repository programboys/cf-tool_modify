package client

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"gitcode.com/sheng_wang/cf-tool_modify/util"
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

// FetchTutorial fetches all Contest materials blog entries for a contest and
// saves each as <title>.md under the contest path.
func (c *Client) Material(info Info, host string) error {
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

	wg := sync.WaitGroup{}
	wg.Add(len(matches))
	mu := sync.Mutex{}

	for _, m := range matches {
		go func(m [][]byte) {
			defer wg.Done()
			blogURL := host + string(m[1])
			linkText := strings.TrimSpace(string(m[2]))
			mu.Lock()
			color.Cyan("Fetching %v (%v)", linkText, blogURL)
			mu.Unlock()

			filename := ""
			if len(info.File) > 0 {
				reg := regexp.MustCompile(`^(.+?)(\.[^.]+)?$`)
				match := reg.FindStringSubmatch(info.File)
				filename = match[1] + "_" + safeName(linkText) + match[2]
			} else {
				filename = safeName(linkText) + ".md"
			}
			err = c.FetchUrlContentToMdFile(blogURL, savePath, filename, &mu)
			mu.Lock()
			if err != nil {
				color.Red("Failed %v: %v", linkText, err)
			} else {
				color.Green("Fetched material for %v  -> %v/%v", blogURL, savePath, filename)
			}
			mu.Unlock()

		}(m)

	}
	wg.Wait()
	return nil
}
