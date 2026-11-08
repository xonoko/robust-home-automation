// Command hub runs the Robust Home Automation hub.
package main

import (
	"os"

	"github.com/xonoko/robust-home-automation/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
