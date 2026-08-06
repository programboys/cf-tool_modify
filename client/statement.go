package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatih/color"
)

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
	ppath := ""
	if len(info.Dir) > 0 {
		ppath = info.Dir
	}
	for _, pid := range problems {
		if len(info.Dir) <= 0 {
			ppath = filepath.Join(contestPath, strings.ToLower(pid))
		}
		go func(pid, ppath string) {
			defer wg.Done()
			if e := os.MkdirAll(ppath, os.ModePerm); e != nil {
				mu.Lock()
				color.Red("Failed %v: %v", pid, e.Error())
				mu.Unlock()
				return
			}
			URL := fmt.Sprintf(urlFormatter, pid)
			fileName := "statement.md"
			if len(info.Dir) > 0 {
				fileName = info.File
			}
			e := c.FetchUrlContentToMdFile(URL, ppath, fileName, &mu)
			mu.Lock()
			if e != nil {
				color.Red("Failed %v: %v", pid, e.Error())
			} else {
				color.Green("Fetched statement for %v  -> %v/%v", URL, ppath, fileName)
			}
			mu.Unlock()
		}(pid, ppath)
	}
	wg.Wait()
	return
}
