package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code, err := Main(ctx, os.Args)
	stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
	}
	os.Exit(code)
}
