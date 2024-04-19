// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"fmt"

	"github.com/mitchellh/cli"
)

type Command struct {
	UI        cli.Ui
	Version   string
	GitCommit string
}

func (c *Command) Run(_ []string) int {
	c.UI.Output(fmt.Sprintf("consul-aws %s", c.Version))
	c.UI.Output(fmt.Sprintf("Git Commit: %s", c.GitCommit))
	return 0
}

func (c *Command) Synopsis() string {
	return "Prints the version"
}

func (c *Command) Help() string {
	return ""
}
