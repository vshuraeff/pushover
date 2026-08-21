package main

import (
	"os"

	"github.com/vshuraeff/pushover/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	cli.Register(root)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
