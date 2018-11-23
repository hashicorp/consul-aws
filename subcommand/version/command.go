package version

import (
	"fmt"

	"github.com/mitchellh/cli"
)

type Command struct {
	UI      cli.Ui
	Version string
}

func (c *Command) Run(_ []string) int {
	c.UI.Output(fmt.Sprintf("consul-aws %s", c.Version))
	return 0
}

func (c *Command) Synopsis() string {
	return "Prints the version"
}

func (c *Command) Help() string {
	return ""
}
