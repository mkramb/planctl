package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/mkramb/planctl/internal/cli"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return cli.Run(ctx, os.Args[1:], cli.Dependencies{
		Dir: ".", Stdout: os.Stdout, Stderr: os.Stderr, Version: version,
	})
}
