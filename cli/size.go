package cli

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/urfave/cli/v3"

	"github.com/dustin/go-humanize"
)

func ParseSize(s string) (uint64, error) {
	return humanize.ParseBytes(s)
}

func FormatSize(s uint64) string {
	return strings.TrimSuffix(humanize.IBytes(s), "B")
}

type (
	SizeFlag = struct {
		cli.FlagBase[uint64, cli.NoConfig, sizeValue[uint64]]
		sizeCompletion
	}
	SizeArg = struct {
		cli.ArgumentBase[uint64, cli.NoConfig, sizeValue[uint64]]
		sizeCompletion
	}
	SizeArgs = struct {
		cli.ArgumentsBase[uint64, cli.NoConfig, sizeValue[uint64]]
		sizeCompletion
	}

	SSizeFlag = struct {
		cli.FlagBase[int64, cli.NoConfig, sizeValue[int64]]
		sizeCompletion
	}
	SSizeArg = struct {
		cli.ArgumentBase[int64, cli.NoConfig, sizeValue[int64]]
		sizeCompletion
	}
	SSizeArgs = struct {
		cli.ArgumentsBase[int64, cli.NoConfig, sizeValue[int64]]
		sizeCompletion
	}
)

var sizeSuffixes = []Suggestion{
	{Text: "B", Note: "Bytes"},
	{Text: "kB", Note: "Kilobytes (1000 bytes)"},
	{Text: "KiB", Note: "Kibibytes (1024 bytes)"},
	{Text: "MB", Note: "Megabytes (1000^2 bytes)"},
	{Text: "MiB", Note: "Mebibytes (1024^2 bytes)"},
	{Text: "GB", Note: "Gigabytes (1000^3 bytes)"},
	{Text: "GiB", Note: "Gibibytes (1024^3 bytes)"},
	{Text: "TB", Note: "Terabytes (1000^4 bytes)"},
	{Text: "TiB", Note: "Tebibytes (1024^4 bytes)"},
	{Text: "PB", Note: "Petabytes (1000^5 bytes)"},
	{Text: "PiB", Note: "Pebibytes (1024^5 bytes)"},
}

type sizeCompletion struct{}

// Complete matches trailing letters against known size suffixes.
func (sizeCompletion) Complete(_ context.Context, completion *Completion) {
	input := completion.Input
	prefix := strings.TrimRightFunc(input, unicode.IsLetter)
	suffix := strings.ToLower(input[len(prefix):])
	for _, unit := range sizeSuffixes {
		if strings.HasPrefix(strings.ToLower(unit.Text), suffix) {
			text, note := input+unit.Text[len(suffix):], unit.Note
			if size, err := ParseSize(text); err == nil {
				note = fmt.Sprintf("%s; %d bytes total", note, size)
			}
			completion.Suggest(text, note)
		}
	}
}

type sizeValue[T int64 | uint64] struct {
	val *T
}

func (sizeValue[T]) Create(value T, dst *T, _ cli.NoConfig) cli.Value {
	if dst == nil {
		dst = new(T)
	}
	*dst = value
	return &sizeValue[T]{val: dst}
}

func (sizeValue[T]) ToString(value T) string {
	// FIXME: Preserve the sign for int64 values here and in String; converting a
	// negative default to uint64 displays a huge positive size.
	return FormatSize(uint64(value))
}

func (v *sizeValue[T]) Set(s string) error {
	// FIXME: Handle signed inputs and check the int64 range before converting.
	// Currently -1Ki is rejected, while 8Ei silently becomes math.MinInt64.
	n, err := ParseSize(s)
	if err != nil {
		return err
	}
	*v.val = T(n)
	return nil
}

func (v *sizeValue[T]) String() string {
	if v == nil || v.val == nil {
		return "0"
	}
	return FormatSize(uint64(*v.val))
}

func (v *sizeValue[T]) Get() any {
	if v == nil || v.val == nil {
		return T(0)
	}
	return *v.val
}
