package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func completionOutputMatches(output []Suggestion, want map[string]string) bool {
	if len(output) != len(want) {
		return false
	}
	got := make(map[string]string, len(output))
	for _, value := range output {
		got[value.Text] = value.Note
	}
	return reflect.DeepEqual(got, want)
}

func TestCompletionPreservesConfiguredSources(t *testing.T) {
	for _, shell := range []string{"bash", "fish", "zsh"} {
		for _, source := range []string{"tools", "trending"} {
			t.Run(shell+"/"+source, func(t *testing.T) {
				configHome := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", configHome)
				if runtime.GOOS == "darwin" {
					t.Setenv("HOME", configHome)
				}
				configDir, err := os.UserConfigDir()
				if err != nil {
					t.Fatal(err)
				}
				configDir = filepath.Join(configDir, "golang-tool-completion")
				if err := os.MkdirAll(configDir, 0o700); err != nil {
					t.Fatal(err)
				}
				configPath := filepath.Join(configDir, source+".txt")
				const content = "example.com/custom\tCustom entry\n"
				if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}

				root := &cli.Command{
					Name:           "go",
					Writer:         io.Discard,
					ErrWriter:      io.Discard,
					Commands:       []*cli.Command{CompletionCommand()},
					ExitErrHandler: func(ctx context.Context, c *cli.Command, err error) {},
				}
				args := []string{"completion", "--disable-" + source, ""}
				if err := MakeCompletion(t.Context(), root, shell, args, nil).Reply(root.Writer); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != content {
					t.Fatalf("completion changed %s: got %q, want %q", configPath, data, content)
				}
			})
		}
	}
}

func TestCompletionInstallCreatesParentDirectories(t *testing.T) {
	dataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)

	var output bytes.Buffer
	if err := doCompletionScript(&output, "bash", "go", "golang-tool-completion", true); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(dataHome, "bash-completion", "completions", "go")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "completion --complete bash") {
		t.Fatalf("installed script %q does not look like bash completion: %q", scriptPath, data)
	}
	if !strings.Contains(output.String(), scriptPath) {
		t.Fatalf("install output %q does not mention %q", output.String(), scriptPath)
	}
}

func TestCompletionStateDoesNotLeakBetweenRequests(t *testing.T) {
	stopOnFirstArg := 1
	var output bytes.Buffer
	root := &cli.Command{
		Name:   "go",
		Writer: &output,
		Commands: []*cli.Command{
			{
				Name:         "run",
				Flags:        []cli.Flag{&cli.BoolFlag{Name: "v", Usage: "verbose"}},
				StopOnNthArg: &stopOnFirstArg,
				Arguments: []cli.Argument{
					&StringArgs{
						Name: "package", Max: -1,
						Completion: func(ctx context.Context, completion *Completion) {
							completion.Suggest(completion.Input+"/", "package")
						},
					},
				},
			},
		},
	}

	if err := MakeCompletion(context.Background(), root, "bash", []string{"run", "example"}, nil).Reply(root.Writer); err != nil {
		t.Fatal(err)
	}
	output.Reset()

	if err := MakeCompletion(context.Background(), root, "bash", []string{"run", "-"}, nil).Reply(root.Writer); err != nil {
		t.Fatal(err)
	}

	if got := output.String(); !strings.Contains(got, "-v") {
		t.Fatalf("second completion = %q, want flag completion", got)
	}
}

func TestCompletionDoesNotRunCommandHandlers(t *testing.T) {
	var output, errOutput bytes.Buffer
	var calls []string
	root := &cli.Command{
		Name:      "sand",
		Writer:    &output,
		ErrWriter: &errOutput,
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			calls = append(calls, "before")
			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
			calls = append(calls, "after")
			return nil
		},
		Commands: []*cli.Command{{
			Name: "configure",
			Arguments: []cli.Argument{&StringArg{
				Name:        "config",
				Suggestions: []Suggestion{{Text: "name:", Note: "Service name"}},
			}},
			Action: func(ctx context.Context, c *cli.Command) error {
				calls = append(calls, "action")
				_, err := fmt.Fprintln(c.Writer, "configured")
				return err
			},
		}},
	}
	if err := MakeCompletion(t.Context(), root, "bash", []string{"configure", ""}, nil).Reply(root.Writer); err != nil {
		t.Fatal(err)
	}
	if output.String() != "name:\n" || errOutput.Len() != 0 || len(calls) != 0 {
		t.Fatalf("completion: stdout %q, stderr %q, handlers %v", output.String(), errOutput.String(), calls)
	}
	output.Reset()
	if err := root.Run(t.Context(), []string{"sand", "configure", "name:example"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "configured\n" || strings.Join(calls, ",") != "before,action,after" {
		t.Fatalf("normal run: stdout %q, handlers %v", output.String(), calls)
	}
}

func TestCallbacksReceiveCompletion(t *testing.T) {
	for _, tt := range []struct {
		name      string
		words     []string
		current   string
		arguments []string
		candidate string
		usage     string
		calls     int
	}{
		{"positional", []string{"configure", "target", "before", "pre"}, "pre", []string{"before", "pre"}, "predone", "match", 1},
		{"flag name", []string{"configure", "--out"}, "--out", nil, "--output", "Output path", 0},
		{"flag value", []string{"configure", "--output", "pre"}, "pre", nil, "predone", "match", 1},
		{"inline flag value", []string{"configure", "--output=pre"}, "pre", nil, "predone", "match", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := &cli.Command{Name: "sand"}
			command := &cli.Command{Name: "configure"}
			root.Commands = []*cli.Command{command}
			var received *Completion
			calls := 0
			onComplete := func(_ context.Context, completion *Completion) {
				calls++
				received = completion
				if completion.Command != command || completion.Shell != "fish" {
					t.Fatalf("callback received another session: %#v", completion)
				}
				if completion.Input != tt.current || !slices.Equal(completion.Arguments, tt.arguments) || !slices.Equal(completion.Words, tt.words) {
					t.Fatalf("callback input: prefix %q, arguments %q, words %q", completion.Input, completion.Arguments, completion.Words)
				}
				completion.Suggest(completion.Input+"done", "match")
			}
			command.Flags = []cli.Flag{&StringFlag{Name: "output", Usage: "Output path", Completion: onComplete}}
			command.Arguments = []cli.Argument{
				&StringArg{Name: "target"},
				&StringArgs{Name: "values", Max: -1, Completion: onComplete},
			}
			session := MakeCompletion(t.Context(), root, "fish", tt.words, nil)
			if calls != tt.calls || calls > 0 && received != session || !completionOutputMatches(session.Output, map[string]string{tt.candidate: tt.usage}) {
				t.Fatalf("completion: %d callback calls, candidates %v", calls, session.Output)
			}
		})
	}
}

func TestCompletionSelectsHandler(t *testing.T) {
	complete := func(label string) CompleteFunc {
		return func(_ context.Context, completion *Completion) {
			completion.Suggest(label+":"+completion.Input, "")
		}
	}
	stopOnFirst := 1
	root := &cli.Command{
		Name: "sand", HideHelp: true, UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&StringFlag{Name: "profile", Aliases: []string{"p"}, Completion: complete("profile")},
			&BoolFlag{Name: "verbose", Aliases: []string{"v"}},
			&StringFlag{Name: "local", Local: true},
		},
		Commands: []*cli.Command{{
			Name: "app", Aliases: []string{"tools"},
			Flags: []cli.Flag{
				&StringFlag{Name: "output", Aliases: []string{"o"}, Completion: complete("output")},
				&cli.IntFlag{Name: "count", Required: true},
				&IntFlag{
					Name:        "limit",
					Suggestions: []Suggestion{{Text: "42", Note: "answer"}},
				},
				&BoolFlag{Name: "hidden", Hidden: true},
				&struct {
					StringFlag
					ValueOptional
				}{
					Name: "fields", Aliases: []string{"columns"},
					Completion: func(_ context.Context, completion *Completion) {
						value := completion.Input
						comma := strings.LastIndex(value, ",") + 1
						selected := strings.Split(value[:comma], ",")
						base, tail := value[:comma], value[comma:]
						for _, field := range []string{"host", "port"} {
							if strings.HasPrefix(field, tail) && !slices.Contains(selected, field) {
								completion.Suggest(base+field, "field")
							}
						}
					},
				},
			},
			Commands: []*cli.Command{
				{Name: "run", Aliases: []string{"r"}, StopOnNthArg: &stopOnFirst, Arguments: []cli.Argument{
					&StringArgs{Name: "package", Max: -1, Completion: complete("package")},
				}},
				{Name: "skip", SkipFlagParsing: true, Arguments: []cli.Argument{
					&StringArgs{Name: "raw", Max: -1, Completion: complete("raw")},
				}},
				{Name: "set", Arguments: []cli.Argument{
					&StringArg{Name: "first", Completion: complete("first")},
					&StringArgs{
						Name: "second", Max: 2,
						Completion: func(_ context.Context, completion *Completion) {
							completion.Suggest(strings.Join(completion.Arguments, "|"), "second")
						},
					},
					&StringArgs{Name: "rest", Max: -1, Completion: complete("rest")},
				}},
				{Name: "native", Arguments: []cli.Argument{
					&cli.IntArg{Name: "number"},
					&StringArgs{Name: "names", Max: 2},
					&StringArgs{Name: "tail", Max: -1, Completion: complete("tail")},
				}},
			},
		}, {Name: "secret", Hidden: true}},
	}
	for _, tt := range []struct {
		name  string
		words []string
		want  map[string]string
	}{
		{"empty", nil, map[string]string{"app": ""}},
		{"command prefix", []string{"ap"}, map[string]string{"app": ""}},
		{"current command", []string{"app"}, map[string]string{"app": ""}},
		{"command alias prefix", []string{"too"}, map[string]string{"tools": ""}},
		{"nested command", []string{"app", "ru"}, map[string]string{"run": ""}},
		{"command aliases", []string{"tools", "r", "x"}, map[string]string{"package:x": ""}},
		{"flag value resembles command", []string{"--profile", "app", "app", "ru"}, map[string]string{"run": ""}},
		{"inline flag before command", []string{"--profile=app", "app", "ru"}, map[string]string{"run": ""}},
		{"boolean flag", []string{"--verbose", "app", "ru"}, map[string]string{"run": ""}},
		{"boolean value", []string{"--verbose=false", "app", "ru"}, map[string]string{"run": ""}},
		{"short flags", []string{"-vp", "app", "app", "ru"}, map[string]string{"run": ""}},
		{"flag name", []string{"app", "--out"}, map[string]string{"--output": ""}},
		{"flag alias", []string{"app", "-o"}, map[string]string{"-o": "", "-output": ""}},
		{"inherited flag name", []string{"app", "--prof"}, map[string]string{"--profile": ""}},
		{"local flag", []string{"app", "--loc"}, map[string]string{}},
		{"hidden flag", []string{"app", "--hid"}, map[string]string{}},
		{"flag argument", []string{"app", "--output", "ru"}, map[string]string{"output:ru": ""}},
		{"flag alias argument", []string{"app", "-o", "ru"}, map[string]string{"output:ru": ""}},
		{"flag argument starts with dash", []string{"app", "--output", "--x"}, map[string]string{"output:--x": ""}},
		{"inline flag argument", []string{"app", "--output=ru"}, map[string]string{"output:ru": ""}},
		{"inline alias argument", []string{"app", "-o=ru"}, map[string]string{"output:ru": ""}},
		{"inherited flag argument", []string{"app", "--profile", "p"}, map[string]string{"profile:p": ""}},
		{"native flag argument", []string{"app", "--count", ""}, map[string]string{}},
		{"custom flag name", []string{"app", "--li"}, map[string]string{"--limit": ""}},
		{"custom flag argument", []string{"app", "--limit", "4"}, map[string]string{"42": "answer"}},
		{"custom inline argument", []string{"app", "--limit=4"}, map[string]string{"42": "answer"}},
		{"invalid flag value", []string{"app", "--count", "invalid", "ru"}, map[string]string{"run": ""}},
		{"unknown flag", []string{"app", "--unknown", "ru"}, map[string]string{"run": ""}},
		{"custom inline completion", []string{"app", "--fields=ho"}, map[string]string{"host": "field"}},
		{"custom field flag name", []string{"app", "--fi"}, map[string]string{"--fields": ""}},
		{"custom field flag alias", []string{"app", "--col"}, map[string]string{"--columns": ""}},
		{"optional field value", []string{"app", "--fields", "ru"}, map[string]string{"run": ""}},
		{"custom inline alias", []string{"app", "--columns=ho"}, map[string]string{"host": "field"}},
		{"empty custom value", []string{"app", "--fields="}, map[string]string{"host": "field", "port": "field"}},
		{"custom comma completion", []string{"app", "--fields=host,po"}, map[string]string{"host,port": "field"}},
		{"custom selected field", []string{"app", "--fields=host,ho"}, map[string]string{}},
		{"delimiter", []string{"app", "--", "ru"}, map[string]string{}},
		{"delimiter as flag value", []string{"app", "--output", "--", "ru"}, map[string]string{"run": ""}},
		{"first argument", []string{"app", "set", "x"}, map[string]string{"first:x": ""}},
		{"bounded argument", []string{"app", "set", "x", "y", "z"}, map[string]string{"y|z": "second"}},
		{"argument boundary", []string{"app", "set", "x", "y", "z", ""}, map[string]string{"rest:": ""}},
		{"interspersed flag", []string{"app", "set", "x", "--output", "out", "y", "z"}, map[string]string{"y|z": "second"}},
		{"native argument bounds", []string{"app", "native", "invalid", "one", "two", "x"}, map[string]string{"tail:x": ""}},
		{"nil callback", []string{"app", "native", "invalid", "one", ""}, map[string]string{}},
		{"negative positional", []string{"app", "native", "-1", "one", "two", "-x"}, map[string]string{"tail:-x": ""}},
		{"stop before positional", []string{"app", "run", "--profile", "value", "--out"}, map[string]string{"--output": ""}},
		{"stop after positional", []string{"app", "run", "pkg", "--out"}, map[string]string{"package:--out": ""}},
		{"skip flag parsing", []string{"app", "skip", "--out"}, map[string]string{"raw:--out": ""}},
		{"skip delimiter parsing", []string{"app", "skip", "--", "x"}, map[string]string{"raw:x": ""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := MakeCompletion(t.Context(), root, "", tt.words, nil)
			if !completionOutputMatches(s.Output, tt.want) {
				t.Fatalf("completion of %q = %v, want %v", tt.words, s.Output, tt.want)
			}
		})
	}
}

func TestCompletionReplyPreservesValues(t *testing.T) {
	root := &cli.Command{
		Name: "sand",
		Flags: []cli.Flag{&StringFlag{
			Name: "output", Aliases: []string{"o"},
			Suggestions: []Suggestion{{Text: "result", Note: "Output path"}},
		}},
	}
	wantResult := map[string]string{"result": "Output path"}
	for _, shell := range []string{"bash", "fish", "zsh"} {
		for _, tt := range []struct {
			name  string
			words []string
			want  string
		}{
			{"separate value", []string{"--output", "re"}, "result"},
			{"inline value", []string{"--output=re"}, "--output=result"},
			{"inline alias", []string{"-o=re"}, "-o=result"},
		} {
			t.Run(shell+"/"+tt.name, func(t *testing.T) {
				completion := MakeCompletion(t.Context(), root, shell, tt.words, nil)
				if !completionOutputMatches(completion.Output, wantResult) {
					t.Fatalf("completed values = %v, want %v", completion.Output, wantResult)
				}
				want := tt.want
				switch shell {
				case "fish":
					want += "\tOutput path"
				case "zsh":
					want += ":Output path"
				}
				want += "\n"
				for range 2 {
					var output strings.Builder
					if err := completion.Reply(&output); err != nil {
						t.Fatal(err)
					}
					if output.String() != want || !completionOutputMatches(completion.Output, wantResult) {
						t.Fatalf("Reply = %q, values = %v; want %q and %v", output.String(), completion.Output, want, wantResult)
					}
				}
			})
		}
	}
}

func TestCompletionReplyPreservesSuggestions(t *testing.T) {
	completion := &Completion{Shell: shellFish}
	completion.Suggest("zeta", "Last")
	completion.Suggest("alpha", "First")
	completion.Suggest("alpha", "Second")
	wantOutput := []Suggestion{
		{Text: "zeta", Note: "Last"},
		{Text: "alpha", Note: "First"},
		{Text: "alpha", Note: "Second"},
	}
	const wantReply = "zeta\tLast\nalpha\tFirst\nalpha\tSecond\n"
	for range 2 {
		var output strings.Builder
		if err := completion.Reply(&output); err != nil {
			t.Fatal(err)
		}
		if output.String() != wantReply || !slices.Equal(completion.Output, wantOutput) {
			t.Fatalf("Reply = %q, values = %v; want %q and %v", output.String(), completion.Output, wantReply, wantOutput)
		}
	}
}

func TestCompletionReplyEscapesBashBraces(t *testing.T) {
	for _, tt := range []struct {
		name, value, addPrefix, cutPrefix, want string
	}{
		{name: "object", value: `{}`, want: `\{}`},
		{name: "nested objects", value: `children:{api:{`, want: `children:\{api:\{`},
		{name: "already escaped", value: `children:\{api:\{`, want: `children:\{api:\{`},
		{name: "escaped backslash", value: `\\{`, want: `\\\{`},
		{name: "double quoted", value: `name:"{"`, want: `name:"{"`},
		{name: "single quoted", value: `name:'{'`, want: `name:'{'`},
		{name: "escaped quote", value: `name:"a\"{"`, want: `name:"a\"{"`},
		{name: "mixed quotes", value: `{name:'{'}`, want: `\{name:'{'}`},
		{name: "other punctuation", value: `[]:},`, want: `[]:},`},
		{name: "prefixes", value: `path/{`, addPrefix: "--output=", cutPrefix: "--output=path/", want: `\{`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := []Suggestion{{Text: tt.value, Note: "Value"}}
			completion := &Completion{Shell: shellBash, AddPrefix: tt.addPrefix, CutPrefix: tt.cutPrefix, Output: slices.Clone(original)}
			for range 2 {
				var output strings.Builder
				if err := completion.Reply(&output); err != nil {
					t.Fatal(err)
				}
				if output.String() != tt.want+"\n" || !slices.Equal(completion.Output, original) {
					t.Fatalf("Reply = %q, values = %v; want %q and %v", output.String(), completion.Output, tt.want+"\n", original)
				}
			}
		})
	}
}

func TestCompletionReplyBraceQuotingAcrossShells(t *testing.T) {
	for _, tt := range []struct{ shell, want string }{
		{"bash", "\\{host:\n"},
		{"fish", "{host:\tServer host\n"},
		{"zsh", "{host\\::Server host\n"},
	} {
		t.Run(tt.shell, func(t *testing.T) {
			completion := &Completion{Shell: tt.shell}
			completion.Suggest("{host:", "Server host")
			var output strings.Builder
			if err := completion.Reply(&output); err != nil {
				t.Fatal(err)
			}
			if output.String() != tt.want || !slices.Equal(completion.Output, []Suggestion{{Text: "{host:", Note: "Server host"}}) {
				t.Fatalf("Reply = %q, values = %v; want %q and an unchanged candidate", output.String(), completion.Output, tt.want)
			}
		})
	}
}

func TestCompletionPreservesCurrentForBashAmbiguity(t *testing.T) {
	values := map[string]string{"a:x": "", "ab:x": ""}
	complete := func(_ context.Context, completion *Completion) {
		for value, usage := range values {
			completion.Suggest(value, usage)
		}
	}
	root := &cli.Command{
		Name: "sand",
		Flags: []cli.Flag{&StringFlag{
			Name: "output", Completion: complete,
		}},
		Arguments: []cli.Argument{&StringArg{
			Name: "value", Completion: complete,
		}},
	}
	for _, tt := range []struct {
		name      string
		shell     string
		words     []string
		addPrefix string
		cutPrefix string
		want      string
	}{
		{"positional", "bash", []string{"x"}, "", "", "a:x\nab:x\nx\n"},
		{"inline", "bash", []string{"--output=x"}, "--output=", "", "--output=a:x\n--output=ab:x\n--output=x\n"},
		{"word breaks", "bash", []string{"--output", "=", "x"}, "", "", "a:x\nab:x\nx\n"},
		{"fish", "fish", []string{"x"}, "", "", "a:x\t\nab:x\t\n"},
		{"zsh", "zsh", []string{"x"}, "", "", "a\\:x:\nab\\:x:\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantResult := map[string]string{"a:x": "", "ab:x": ""}
			if tt.shell == "bash" {
				wantResult["x"] = ""
			}
			completion := MakeCompletion(t.Context(), root, tt.shell, tt.words, nil)
			if !completionOutputMatches(completion.Output, wantResult) {
				t.Fatalf("completed values = %v, want %v", completion.Output, wantResult)
			}
			if completion.AddPrefix != tt.addPrefix || completion.CutPrefix != tt.cutPrefix {
				t.Fatalf("prefixes = (%q, %q), want (%q, %q)", completion.AddPrefix, completion.CutPrefix, tt.addPrefix, tt.cutPrefix)
			}
			for range 2 {
				var output strings.Builder
				if err := completion.Reply(&output); err != nil {
					t.Fatal(err)
				}
				if output.String() != tt.want || !completionOutputMatches(completion.Output, wantResult) {
					t.Fatalf("Reply = %q, values = %v; want %q and %v", output.String(), completion.Output, tt.want, wantResult)
				}
			}
		})
	}
}

func TestBashFixupAccountsForCutPrefix(t *testing.T) {
	completion := &Completion{
		Shell: "bash", Input: "path/x",
		AddPrefix: "--output=", CutPrefix: "--output=path/",
	}
	completion.Suggest("path/a:x", "")
	completion.Suggest("path/ab:x", "")
	completion.Suggest("other", "")
	completion.fixupOutput()
	wantResult := map[string]string{"path/a:x": "", "path/ab:x": "", "path/x": "", "other": ""}
	if !completionOutputMatches(completion.Output, wantResult) || completion.AddPrefix != "--output=" || completion.CutPrefix != "--output=path/" {
		t.Fatalf("Bash fixup = %#v, want completed values including Input and unchanged prefixes", completion)
	}
	for range 2 {
		var output strings.Builder
		if err := completion.Reply(&output); err != nil {
			t.Fatal(err)
		}
		if output.String() != "a:x\nab:x\nx\n" || !completionOutputMatches(completion.Output, wantResult) {
			t.Fatalf("Reply = %q, values = %v; want the current replacement and unchanged values", output.String(), completion.Output)
		}
	}
}

func TestBashFlagWordBreaks(t *testing.T) {
	root := &cli.Command{
		Name: "sand",
		Flags: []cli.Flag{&StringFlag{
			Name: "output", Aliases: []string{"o"},
			Suggestions: []Suggestion{{Text: "result"}},
		}},
	}
	for _, words := range [][]string{
		{"--output", "=", "re"}, {"--output", "="}, {"-o", "=", "re"},
		{"--output", "=", "previous", "--output", "=", "re"},
	} {
		t.Run(strings.Join(words, " "), func(t *testing.T) {
			original := append([]string(nil), words...)
			completion := MakeCompletion(t.Context(), root, "bash", words, nil)
			wantResult := map[string]string{"result": ""}
			if !completionOutputMatches(completion.Output, wantResult) {
				t.Fatalf("completed values = %v, want %v", completion.Output, wantResult)
			}
			if completion.AddPrefix != "" || completion.CutPrefix != "" {
				t.Fatalf("Bash prefixes: add %q, cut %q; want both empty after cancellation", completion.AddPrefix, completion.CutPrefix)
			}
			for range 2 {
				var output strings.Builder
				if err := completion.Reply(&output); err != nil {
					t.Fatal(err)
				}
				if output.String() != "result\n" || !reflect.DeepEqual(words, original) || !completionOutputMatches(completion.Output, wantResult) {
					t.Fatalf("completion = %q, words = %v, values = %v; want result and unchanged input and values", output.String(), words, completion.Output)
				}
			}
		})
	}
}

func TestValueCompletionPreservesFlagActions(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "values"
		if missing {
			name = "required flag"
		}
		t.Run(name, func(t *testing.T) {
			var output string
			var enabled bool
			var items []string
			actions := 0
			completion := func(context.Context, *Completion) { t.Fatal("completion called during normal parsing") }
			flag := &StringFlag{
				Name: "output", Aliases: []string{"o"}, Required: true, Destination: &output,
				Action: func(context.Context, *cli.Command, string) error {
					actions++
					return nil
				},
				Completion: completion,
			}
			argument := &StringArgs{
				Name: "items", Min: 1, Max: 2, Destination: &items,
				Completion: completion,
			}
			root := &cli.Command{
				Name: "sand", Writer: io.Discard, ErrWriter: io.Discard,
				ExitErrHandler: func(context.Context, *cli.Command, error) {},
				Flags: []cli.Flag{
					flag,
					&BoolFlag{Name: "enabled", Destination: &enabled},
				},
				Commands: []*cli.Command{{
					Name: "configure", Arguments: []cli.Argument{argument},
					Action: func(_ context.Context, command *cli.Command) error {
						if command.String("output") != "result" || command.Count("output") != 1 || !command.Bool("enabled") {
							t.Fatalf("wrapped flag values: output %q, count %d, enabled %v", command.String("output"), command.Count("output"), command.Bool("enabled"))
						}
						return nil
					},
				}},
			}
			args := []string{"sand", "configure"}
			if !missing {
				args = append(args, "-o", "result", "--enabled", "one", "two")
			}
			err := root.Run(t.Context(), args)
			if missing {
				if err == nil || !strings.Contains(err.Error(), "output") {
					t.Fatalf("missing required flag: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if output != "result" || !enabled || actions != 1 || !flag.IsSet() || !slices.Equal(items, []string{"one", "two"}) {
				t.Fatalf("parsed values: output %q, enabled %v, actions %d, is set %v, items %v", output, enabled, actions, flag.IsSet(), items)
			}
			value, ok := argument.Get().([]string)
			if !ok || !slices.Equal(value, items) || !argument.HasName("items") || argument.Usage() == "" {
				t.Fatalf("wrapped argument metadata: name %v, usage %q, value %v", argument.HasName("items"), argument.Usage(), argument.Get())
			}
		})
	}
}

func TestValueCompletionPreservesParsing(t *testing.T) {
	var count int
	var tags, values []string
	completion := func(context.Context, *Completion) { t.Fatal("completion called during normal parsing") }
	command := Command{
		Name: "sand", SliceFlagSeparator: ";", Writer: io.Discard, ErrWriter: io.Discard,
		Flags: []Flag{
			&IntFlag{Name: "count", Destination: &count, Completion: completion},
			&StringSliceFlag{Name: "tag", Destination: &tags, Completion: completion},
		},
		Arguments: []Argument{&StringArgs{Name: "values", Max: 2, Destination: &values, Completion: completion}},
		Action:    func(context.Context, *Command) error { return nil },
	}
	if err := command.Run(t.Context(), []string{"sand", "--count", "7", "--tag", "first;second", "one", "two"}); err != nil {
		t.Fatal(err)
	}
	if count != 7 || !slices.Equal(tags, []string{"first", "second"}) || !slices.Equal(values, []string{"one", "two"}) {
		t.Fatalf("parsed values: count %d, tags %v, values %v", count, tags, values)
	}
}
