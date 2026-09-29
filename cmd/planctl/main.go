package main

import (
	"context"
	"os"
	"os/signal"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return Run(ctx, os.Args[1:], Dependencies{
		Dir: ".", Stdout: os.Stdout, Stderr: os.Stderr, Version: version,
	})
}
