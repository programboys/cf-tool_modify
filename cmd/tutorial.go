package cmd

import (
	"errors"

	"gitcode.com/sheng_wang/cf-tool_modify/client"
	"gitcode.com/sheng_wang/cf-tool_modify/config"
	"github.com/fatih/color"
)

// Material command
func Material() (err error) {
	cln := client.Instance
	info := Args.Info
	if info.ContestID == "" {
		return errors.New("You have to specify the Contest ID")
	}

	err = cln.FetchTutorial(info, config.Instance.Host)
	if err != nil {
		if err = loginAgain(cln, err); err == nil {
			err = cln.FetchTutorial(info, config.Instance.Host)
		}
	}
	if err != nil {
		color.Red(err.Error())
	}
	return
}
