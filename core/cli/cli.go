package cli

import (
	"github.com/urfave/cli/v3"
)

type (
	Command = cli.Command

	Flag                = cli.Flag
	IntFlag             = cli.IntFlag
	Int64Flag           = cli.Int64Flag
	UintFlag            = cli.UintFlag
	Uint64Flag          = cli.Uint64Flag
	BoolFlag            = cli.BoolFlag
	BoolWithInverseFlag = cli.BoolWithInverseFlag
	StringFlag          = cli.StringFlag
	StringSliceFlag     = cli.StringSliceFlag
	DurationFlag        = cli.DurationFlag

	Argument      = cli.Argument
	IntArg        = cli.IntArg
	Int64Arg      = cli.Int64Arg
	UintArg       = cli.UintArg
	Uint64Arg     = cli.Uint64Arg
	StringArg     = cli.StringArg
	StringMapArgs = cli.StringMapArgs
	StringArgs    = cli.StringArgs

	ExitCoder = cli.ExitCoder
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
