package cli_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/koct9i/sand/cli"
)

func TestSizeCompletion(t *testing.T) {
	root := &cli.Command{
		Name: "sand",
		Flags: []cli.Flag{
			&cli.SizeFlag{Name: "size", Aliases: []string{"s"}},
			&cli.SSizeFlag{Name: "signed-size"},
		},
		Commands: []*cli.Command{
			{Name: "one", Arguments: []cli.Argument{&cli.SizeArg{Name: "size"}}},
			{Name: "many", Arguments: []cli.Argument{&cli.SizeArgs{Name: "sizes", Max: -1}}},
			{Name: "signed-one", Arguments: []cli.Argument{&cli.SSizeArg{Name: "size"}}},
			{Name: "signed-many", Arguments: []cli.Argument{&cli.SSizeArgs{Name: "sizes", Max: -1}}},
		},
	}
	for _, tt := range []struct {
		name  string
		words []string
		want  []string
	}{
		{"number", []string{"--size", "1"}, []string{"1B", "1GB", "1GiB", "1KiB", "1MB", "1MiB", "1PB", "1PiB", "1TB", "1TiB", "1kB"}},
		{"partial suffix", []string{"--size", "2M"}, []string{"2MB", "2MiB"}},
		{"lowercase suffix", []string{"--size", "2m"}, []string{"2mB", "2miB"}},
		{"unicode suffix", []string{"--size", "2K"}, []string{"2KB", "2KiB"}},
		{"decimal", []string{"--size", "1.5Ki"}, []string{"1.5KiB"}},
		{"grouped number", []string{"--size", "1,024G"}, []string{"1,024GB", "1,024GiB"}},
		{"space before suffix", []string{"--size", "2 Mi"}, []string{"2 MiB"}},
		{"complete suffix", []string{"--size", "2MiB"}, []string{"2MiB"}},
		{"empty", []string{"--size", ""}, []string{"B", "GB", "GiB", "KiB", "MB", "MiB", "PB", "PiB", "TB", "TiB", "kB"}},
		{"suffix only", []string{"--size", "M"}, []string{"MB", "MiB"}},
		{"prefix preserved", []string{"--size", "1..2Mi"}, []string{"1..2MiB"}},
		{"invalid suffix", []string{"--size", "2XB"}, nil},
		{"large prefix", []string{"--size", "16384Pi"}, []string{"16384PiB"}},
		{"alias", []string{"-s", "2M"}, []string{"2MB", "2MiB"}},
		{"inline flag", []string{"--size=2M"}, []string{"2MB", "2MiB"}},
		{"bash word break", []string{"--size", "=", "2M"}, []string{"2MB", "2MiB"}},
		{"signed flag", []string{"--signed-size", "2M"}, []string{"2MB", "2MiB"}},
		{"signed prefix", []string{"--signed-size", "-2Mi"}, []string{"-2MiB"}},
		{"argument", []string{"one", "2M"}, []string{"2MB", "2MiB"}},
		{"repeated argument", []string{"many", "1KiB", "2M"}, []string{"2MB", "2MiB"}},
		{"signed argument", []string{"signed-one", "2M"}, []string{"2MB", "2MiB"}},
		{"signed repeated argument", []string{"signed-many", "1KiB", "2M"}, []string{"2MB", "2MiB"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			completion := cli.MakeCompletion(t.Context(), root, "bash", tt.words, nil)
			var got []string
			for _, suggestion := range completion.Output {
				got = append(got, suggestion.Text)
				if suggestion.Note == "" {
					t.Errorf("suggestion %q has no unit description", suggestion.Text)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("completion of %q = %q, want %q", tt.words, got, tt.want)
			}
		})
	}

	completion := cli.MakeCompletion(t.Context(), root, "bash", []string{"--size=1Mi"}, nil)
	var output strings.Builder
	if err := completion.Reply(&output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "--size=1MiB\n"; got != want {
		t.Fatalf("inline reply = %q, want %q", got, want)
	}
}

func TestSizeCompletionPreservesParsing(t *testing.T) {
	var unsignedFlag, unsignedArg uint64
	var signedFlag, signedArg int64
	var unsignedArgs []uint64
	var signedArgs []int64
	root := &cli.Command{
		Name: "sand", Writer: io.Discard, ErrWriter: io.Discard,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Flags: []cli.Flag{
			&cli.SizeFlag{Name: "size", Destination: &unsignedFlag},
			&cli.SSizeFlag{Name: "signed-size", Destination: &signedFlag},
		},
		Arguments: []cli.Argument{
			&cli.SizeArg{Name: "one", Destination: &unsignedArg},
			&cli.SSizeArg{Name: "signed-one", Destination: &signedArg},
			&cli.SizeArgs{Name: "many", Max: 2, Destination: &unsignedArgs},
			&cli.SSizeArgs{Name: "signed-many", Max: -1, Destination: &signedArgs},
		},
		Action: func(context.Context, *cli.Command) error { return nil },
	}
	if err := root.Run(t.Context(), []string{"sand", "--size", "1MiB", "--signed-size", "2MB", "3KiB", "4kB", "5MB", "6MiB", "7GB", "8GiB"}); err != nil {
		t.Fatal(err)
	}
	if unsignedFlag != 1<<20 || signedFlag != 2_000_000 || unsignedArg != 3<<10 || signedArg != 4_000 ||
		!slices.Equal(unsignedArgs, []uint64{5_000_000, 6 << 20}) || !slices.Equal(signedArgs, []int64{7_000_000_000, 8 << 30}) {
		t.Fatalf("parsed sizes: flags %d, %d; arguments %d, %d, %v, %v", unsignedFlag, signedFlag, unsignedArg, signedArg, unsignedArgs, signedArgs)
	}
}
