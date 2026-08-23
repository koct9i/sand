package cli

import (
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/dustin/go-humanize"
)

func ParseSize(s string) (uint64, error) {
	return humanize.ParseBytes(s)
}

func FormatSize(s uint64) string {
	return strings.TrimSuffix(humanize.IBytes(s), "B")
}

type SizeFlag = cli.FlagBase[uint64, cli.NoConfig, sizeValue]
type SizeArg = cli.ArgumentBase[uint64, cli.NoConfig, sizeValue]
type SizeArgs = cli.ArgumentsBase[uint64, cli.NoConfig, sizeValue]

type sizeValue struct {
	val *uint64
}

func (sizeValue) Create(value uint64, dst *uint64, _ cli.NoConfig) cli.Value {
	if dst == nil {
		dst = new(uint64)
	}
	*dst = value
	return &sizeValue{val: dst}
}

func (sizeValue) ToString(value uint64) string {
	return FormatSize(value)
}

func (v *sizeValue) Set(s string) error {
	n, err := ParseSize(s)
	if err != nil {
		return err
	}
	*v.val = uint64(n)
	return nil
}

func (v *sizeValue) String() string {
	if v == nil || v.val == nil {
		return "0"
	}
	return FormatSize(*v.val)
}

func (v *sizeValue) Get() any {
	if v == nil || v.val == nil {
		return uint64(0)
	}
	return *v.val
}
