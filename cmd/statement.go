package cmd

import (
	"github.com/xalanq/cf-tool/client"
)

// Statement command
func Statement() (err error) {
	cln := client.Instance
	info := Args.Info
	work := func() error {
		return cln.Statement(info)
	}
	if err = work(); err != nil {
		if err = loginAgain(cln, err); err == nil {
			err = work()
		}
	}
	return
}
