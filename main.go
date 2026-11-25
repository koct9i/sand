package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/koct9i/sand/log"

	"github.com/koct9i/sand/cli"

	"github.com/koct9i/sand/app/ginkgo"
	"github.com/koct9i/sand/app/hello"
	"github.com/koct9i/sand/app/serve"
	"github.com/koct9i/sand/app/sleep"
	"github.com/koct9i/sand/app/ssh"
	"github.com/koct9i/sand/app/store"
	"github.com/koct9i/sand/example/completion"
)

type jsonrServer struct {
	Host  string `json:"host" help:"Server host"`
	Ports []int  `json:"ports" help:"Listening ports"`
}

type jsonrConfig struct {
	Name    string                 `json:"name" help:"Service name"`
	Enabled bool                   `json:"enabled" help:"Enable service"`
	Server  jsonrServer            `json:"server" help:"Server settings"`
	Servers []jsonrServer          `json:"servers"`
	ByName  map[string]jsonrServer `json:"byName"`
	Spaced  string                 `json:"two words" help:"Quoted field"`
	Hidden  string                 `json:"-"`
}

func Main(ctx context.Context, args []string) (int, error) {
	var logger logr.Logger
	var timeout time.Duration
	var ctxCancel func()

	var config jsonrConfig

	command := cli.Command{
		UseShortOptionHandling: true,
		HideHelpCommand:        true,
		ExitErrHandler:         func(ctx context.Context, c *cli.Command, err error) {},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:        "log-verbosity",
				Aliases:     []string{"v"},
				Destination: &log.Verbosity,
			},
			&cli.BoolWithInverseFlag{
				Name:        "log-pretty",
				Value:       true,
				Destination: &log.Pretty,
			},
			&cli.DurationFlag{
				Name:        "timeout",
				Aliases:     []string{"t"},
				Destination: &timeout,
			},
		},
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			if timeout > 0 {
				ctx, ctxCancel = context.WithTimeout(ctx, timeout)
			}
			logger = log.NewLogger(os.Stderr)
			ctx = logr.NewContext(ctx, logger)
			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
			if ctxCancel != nil {
				ctxCancel()
			}
			return nil
		},
		Commands: []*cli.Command{
			cli.CompletionCommand(),
			{
				Name:  "example",
				Usage: "run examples",
				Commands: []*cli.Command{
					completion.NewCommand(),
				},
			},
			{
				Name: "json",
				Arguments: []cli.Argument{
					&cli.RelaxedJSONArguments[jsonrConfig]{
						Name: "config", 
						Destination: &config,
					},
				},
			},
			{
				Name: "app",
				Commands: []*cli.Command{
					ginkgo.NewCommand(),
					{
						Name: "hello",
						Action: func(ctx context.Context, c *cli.Command) error {
							return hello.Main(ctx, c.Args().Slice())
						},
						SkipFlagParsing: true,
					},
					{
						Name: "sleep",
						Flags: []cli.Flag{
							&cli.DurationFlag{
								Name:  "t",
								Value: time.Second,
							},
						},
						Action: func(ctx context.Context, c *cli.Command) error {
							return sleep.Main(ctx, c.Duration("t"))
						},
					},
					{
						Name: "start-systemd",
						Action: func(ctx context.Context, c *cli.Command) error {
							return serve.StartSystemd(ctx)
						},
					},
					{
						Name: "stop-systemd",
						Action: func(ctx context.Context, c *cli.Command) error {
							return serve.StopSystemd(ctx)
						},
					},
					{
						Name: "serve-systemd",
						Action: func(ctx context.Context, c *cli.Command) error {
							return serve.ServeSystemd(ctx)
						},
					},
					{
						Name: "serve",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name: "systemd-socket",
							},
							&cli.StringFlag{
								Name:  "address",
								Value: "localhost:8080",
							},
						},
						Action: func(ctx context.Context, c *cli.Command) error {
							return serve.Main(ctx, c.String("address"))
						},
					},
					{
						Name: "ssh",
						Arguments: []cli.Argument{
							&cli.StringArg{
								Name:  "address",
								Value: "localhost",
							},
							&cli.StringArgs{
								Name: "command",
								Min:  1,
								Max:  -1,
							},
						},
						Action: func(ctx context.Context, c *cli.Command) error {
							return ssh.Main(ctx, c.StringArg("address"), c.StringArgs("command"))
						},
						SkipFlagParsing: true,
					},
					{
						Name:      "store",
						Arguments: []cli.Argument{},
						Action: func(ctx context.Context, c *cli.Command) error {
							return store.Main(ctx)
						},
					},
				},
			},
		},
	}
	args = append(strings.Split(args[0], "__"), os.Args[1:]...)
	if err := command.Run(ctx, args); err != nil {
		exitCode := 1
		if exitCoder, ok := errors.AsType[cli.ExitCoder](err); ok {
			exitCode = exitCoder.ExitCode()
		}
		if exitCode != 0 {
			logger.Error(err, "Failed")
		}
		return exitCode, err
	}
	return 0, nil
}
