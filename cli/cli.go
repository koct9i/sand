package cli

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/koct9i/sand/app/hello"
	"github.com/koct9i/sand/app/sleep"
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
				},
			},
		},
	}
	if err := command.Run(ctx, args); err != nil {
		if exitErr, ok := err.(cli.ExitCoder); ok {
			return exitErr.ExitCode(), err
		}
		return 1, err
	}
	return 0, nil
}
