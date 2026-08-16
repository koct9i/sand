package cli

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"

	"github.com/koct9i/sand/app/hello"
	"github.com/koct9i/sand/app/serve"
	"github.com/koct9i/sand/app/sleep"
	"github.com/koct9i/sand/app/ssh"
	"github.com/koct9i/sand/app/store"
)

type NoMoreArguments struct {
	cli.StringArgs
}

func (a *NoMoreArguments) Parse(s []string) ([]string, error) {
	if len(s) > 0 {
		return s, cli.Exit("no more arguments are expected", 1)
	}
	return s, nil
}

var NoArguments = []cli.Argument{
	&NoMoreArguments{},
}

func Main(ctx context.Context, args []string) (int, error) {
	command := cli.Command{
		ExitErrHandler: func(ctx context.Context, c *cli.Command, err error) {},
		Commands: []*cli.Command{
			{
				Name: "app",
				Commands: []*cli.Command{
					{
						Name:            "hello",
						SkipFlagParsing: true,
						Action: func(ctx context.Context, c *cli.Command) error {
							return hello.Main(ctx, c.Args().Slice())
						},
					},
					{
						Name: "sleep",
						Flags: []cli.Flag{
							&cli.DurationFlag{
								Name: "t",
							},
						},
						Arguments: NoArguments,
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
								Name:  "command",
								Min: 1,
								Max: -1,
							},
						},
						Action: func(ctx context.Context, c *cli.Command) error {
							return ssh.Main(ctx, c.StringArg("address"), c.StringArgs("command"))
						},
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
	if err := command.Run(ctx, args); err != nil {
		var exitCoder cli.ExitCoder
		if errors.As(err, &exitCoder) {
			return exitCoder.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}
