package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/vshuraeff/pushover/internal/cli"
)

func main() {
	root := cli.NewRootCommand()
	cli.Register(root)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := root.ExecuteContext(ctx)
	stop()
	if err != nil {
		os.Exit(1)
	}
}
