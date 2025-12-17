package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/koct9i/sand/cli"
)

func main() {
        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
        defer stop()
		if code, err := cli.Main(ctx, os.Args); err != nil {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
			os.Exit(code)
		}
}
