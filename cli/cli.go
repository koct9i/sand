package cli

import (
	"context"
	"strings"

	"github.com/urfave/cli/v3"
)

type (
	Command = cli.Command

	Flag = cli.Flag

	IntFlag = struct {
		cli.IntFlag
		ValueCompletion
	}
	Int64Flag = struct {
		cli.Int64Flag
		ValueCompletion
	}
	UintFlag = struct {
		cli.UintFlag
		ValueCompletion
	}
	Uint64Flag = struct {
		cli.Uint64Flag
		ValueCompletion
	}
	Float32Flag = struct {
		cli.Float32Flag
		ValueCompletion
	}
	Float64Flag = struct {
		cli.Float64Flag
		ValueCompletion
	}
	BoolFlag = struct {
		cli.BoolFlag
		ValueCompletion
	}
	BoolWithInverseFlag = struct {
		cli.BoolWithInverseFlag
		ValueCompletion
	}
	StringFlag = struct {
		cli.StringFlag
		ValueCompletion
	}
	StringSliceFlag = struct {
		cli.StringSliceFlag
		ValueCompletion
	}
	StringMapFlag = struct {
		cli.StringMapFlag
		ValueCompletion
	}
	DurationFlag = struct {
		cli.DurationFlag
		ValueCompletion
	}
	GenericFlag = struct {
		cli.GenericFlag
		ValueCompletion
	}

	Argument = cli.Argument

	IntArg = struct {
		cli.IntArg
		ValueCompletion
	}
	Int64Arg = struct {
		cli.Int64Arg
		ValueCompletion
	}
	UintArg = struct {
		cli.UintArg
		ValueCompletion
	}
	Uint64Arg = struct {
		cli.Uint64Arg
		ValueCompletion
	}
	Float32Arg = struct {
		cli.Float32Arg
		ValueCompletion
	}
	Float64Arg = struct {
		cli.Float64Arg
		ValueCompletion
	}
	StringArg = struct {
		cli.StringArg
		ValueCompletion
	}
	StringMapArgs = struct {
		cli.StringMapArgs
		ValueCompletion
	}
	StringArgs = struct {
		cli.StringArgs
		ValueCompletion
	}

	ExitCoder = cli.ExitCoder
)

var (
	HelpFlag = BoolFlag{
		Name:    "help",
		Aliases: []string{"h"},
		Usage:   "show help",
		Action: func(ctx context.Context, c *Command, b bool) (err error) {
			if c == c.Root() {
				err = cli.ShowRootCommandHelp(c)
			} else {
				err = cli.ShowSubcommandHelp(c)
			}
			if err != nil {
				return err
			}
			return Exit("", 0)
		},
	}
)

func Exit(message any, exitCode int) ExitCoder {
	return cli.Exit(message, exitCode)
}

func EnvVars(keys ...string) cli.ValueSourceChain {
	return cli.EnvVars(keys...)
}

type NoMoreArguments struct {
	StringArgs
}

func (a *NoMoreArguments) Parse(s []string) ([]string, error) {
	if len(s) > 0 {
		return s, Exit("no more arguments are expected", 1)
	}
	return s, nil
}

var NoArguments = []Argument{
	&NoMoreArguments{},
}

type ValueCompletion struct {
	Completion  CompleteFunc
	Suggestions []Suggestion
}

func (v *ValueCompletion) Complete(ctx context.Context, completion *Completion) {
	if v.Completion != nil {
		v.Completion(ctx, completion)
	}
	for _, s := range v.Suggestions {
		if strings.HasPrefix(s.Text, completion.Input) {
			completion.Suggest(s.Text, s.Note)
		}
	}
}

type ValueOptional struct{}

// IsBoolFlag lets a bare --flag be parsed as --flag=true.
func (*ValueOptional) IsBoolFlag() bool {
	return true
}
