package cli

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/urfave/cli/v3"
)

const (
	shellBash = "bash"
	shellFish = "fish"
	shellZsh  = "zsh"
)

type CompleteFunc func(context.Context, *Completion)

type Completor interface {
	Complete(ctx context.Context, completion *Completion)
}

type Suggestion struct {
	Text string
	Note string
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

type BoolCompletion struct {
	TNote string
	FNote string
}

func (v *BoolCompletion) Complete(_ context.Context, completion *Completion) {
	value, err := strconv.ParseBool(completion.Input)
	if err != nil || value {
		completion.Suggest("true", cmp.Or(v.TNote, "enable"))
	}
	if err != nil || !value {
		completion.Suggest("false", cmp.Or(v.FNote, "disable"))
	}
}

// Completion holds the input, parser state, and results of one completion request.
// Completion handlers read Input and Arguments and add candidates using Suggest.
// For flag values, Input and Output exclude the flag name. Reply applies
// AddPrefix and CutPrefix to produce the text the shell will replace.
type Completion struct {
	Shell string
	// Words excludes the executable and ends with the word at the cursor, which
	// may be empty. Bash's flag/value word breaks are normalized before parsing.
	Words []string

	DebugWriter io.Writer

	Command *Command
	// Commands runs from the root to Command without relying on initialized parents.
	Commands []*Command
	// Flags includes inherited and hidden flags; visibility is checked when suggesting names.
	Flags []Flag
	// Positional holds completed words and is narrowed during argument dispatch.
	Positional []string
	// Pending expects a separate flag value, even if that word looks like a flag or command.
	Pending   Flag
	StopFlags bool

	// AddPrefix is prepended to completed values by Reply, for example "--output=".
	AddPrefix string
	// CutPrefix is removed after AddPrefix is added, matching the shell's replacement
	// span. Reply omits candidates that do not start with CutPrefix.
	CutPrefix string

	Complete CompleteFunc
	// Arguments contains words for the active positional argument, including Input.
	// It is not populated for flag value completion.
	Arguments []string

	// Input is the current text being completed.
	Input string
	// Output holds suggested completion texts and notes in suggestion order.
	Output []Suggestion
}

// MakeCompletion resolves the active command and handler, then collects candidates.
// words excludes the executable; append an empty word when completing after a space.
// During resolution flag and argument values are not parsed, actions are not called.
// Output suggestions are sorted in alphabetical order.
func MakeCompletion(ctx context.Context, root *Command, shell string, words []string, debug io.Writer) *Completion {
	s := &Completion{
		Shell:       shell,
		Words:       slices.Clone(words),
		DebugWriter: debug,
	}
	s.Debug("completion: request shell=%q words=%q\n", shell, words)

	s.normalizeWords()

	if len(s.Words) == 0 {
		s.Words = []string{""}
	}
	s.Debug("completion: normalized words=%q cut_prefix=%q\n", s.Words, s.CutPrefix)

	s.Input = s.Words[len(s.Words)-1]

	s.enterCommand(root)

	s.parseWords()
	s.Debug("completion: parsed input=%q positional=%q stop_flags=%t add_prefix=%q cut_prefix=%q\n", s.Input, s.Positional, s.StopFlags, s.AddPrefix, s.CutPrefix)

	if s.Complete != nil {
		s.Complete(ctx, s)
	} else {
		s.Debug("completion: no completion handler\n")
	}

	s.fixupOutput()

	slices.SortStableFunc(s.Output, func(a, b Suggestion) int {
		return strings.Compare(a.Text, b.Text)
	})
	s.Debug("completion: collected %d candidates\n", len(s.Output))

	return s
}

// Suggest appends a completed value and its description.
func (s *Completion) Suggest(text, note string) {
	s.Debug("completion: suggest text=%q note=%q\n", text, note)
	s.Output = append(s.Output, Suggestion{Text: text, Note: note})
}

// Debug writes formatted diagnostics to DebugWriter when completion debugging is enabled.
//
//nolint:goprintffuncname // Preserve the public callback API.
func (s *Completion) Debug(format string, a ...any) {
	if s.DebugWriter != nil {
		_, _ = fmt.Fprintf(s.DebugWriter, format, a...) //nolint:errcheck // Diagnostics must not interrupt completion.
	}
}

// Reply applies AddPrefix/CutPrefix and shell quoting to completion suggestions.
func (s *Completion) Reply(w io.Writer) error {
	// FIXME: Normalize multiline notes and escape or reject embedded newlines in
	// candidate text and Fish field tabs. Writing them verbatim turns one candidate
	// or note into multiple protocol records.
	s.Debug("completion: reply shell=%q candidates=%d add_prefix=%q cut_prefix=%q\n", s.Shell, len(s.Output), s.AddPrefix, s.CutPrefix)

	buffer := bufio.NewWriter(w)
	edit := func(value string) (string, bool) {
		text, ok := strings.CutPrefix(s.AddPrefix+value, s.CutPrefix)
		if ok && s.Shell == shellBash {
			text = escapeBashCompletion(text)
		}
		return text, ok
	}

	// FIXME: Deduplicate Bash replacement texts before counting and adding notes.
	// Duplicate "true" suggestions with different notes can make Readline insert
	// the shared description prefix, such as "true   (", into the command line.
	width, count := 0, 0
	if s.Shell == shellBash {
		for _, value := range s.Output {
			if suggest, ok := edit(value.Text); ok {
				width = max(width, len(suggest))
				count++
			}
		}
	}

	for _, value := range s.Output {
		text, ok := edit(value.Text)
		if !ok {
			s.Debug("completion: skip text=%q: does not match cut_prefix=%q\n", value.Text, s.CutPrefix)
			continue
		}
		var err error
		note := value.Note
		switch {
		case s.Shell == shellBash && note != "" && count > 1:
			// Add description if have many suggestions, only common prefix will be used.
			_, err = fmt.Fprintf(buffer, "%*s (%s)\n", -width-2, text, note)
		case s.Shell == shellFish:
			_, err = fmt.Fprintf(buffer, "%s\t%s\n", text, note)
		case s.Shell == shellZsh:
			// _describe separates the candidate and description with a colon.
			text = strings.NewReplacer("\\", "\\\\", ":", "\\:").Replace(text)
			_, err = fmt.Fprintf(buffer, "%s:%s\n", text, note)
		default:
			_, err = fmt.Fprintln(buffer, text)
		}
		if err != nil {
			return err
		}
		s.Debug("completion: reply text=%q note=%q\n", text, note)
	}

	if err := buffer.Flush(); err != nil {
		return err
	}

	return nil
}

// escapeBashCompletion protects unquoted shell metacharacters in completion text.
// Fish and Zsh quote candidates when inserting them; Bash inserts our text verbatim.
func escapeBashCompletion(text string) string {
	var out strings.Builder
	var quote byte
	start := 0
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\\' && quote != '\'':
			i++ // Keep an existing escape and its following byte together.
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.ContainsRune(" \t<>()&;|{*?[$`!#~", rune(c)):
			out.WriteString(text[start:i])
			out.WriteByte('\\')
			out.WriteByte(c)
			start = i + 1
		}
	}
	if start == 0 {
		return text
	}
	out.WriteString(text[start:])
	return out.String()
}

func completeFlagNames(_ context.Context, completion *Completion) {
	prefix := completion.Input
	namePrefix := strings.TrimLeft(prefix, "-")
	long := strings.HasPrefix(prefix, "--")
	for _, flag := range completion.Flags {
		if f, ok := flag.(cli.VisibleFlag); ok && !f.IsVisible() {
			continue
		}
		usage := ""
		if f, ok := flag.(interface{ GetUsage() string }); ok {
			usage = f.GetUsage()
		}
		for _, name := range flag.Names() {
			if !strings.HasPrefix(name, namePrefix) || len(name) == 1 && long {
				continue
			}
			dash := "-"
			if long || prefix == "-" && len(name) > 1 {
				dash = "--"
			}
			completion.Suggest(dash+name, usage)
		}
	}
}

func completeCommands(_ context.Context, completion *Completion) {
	prefix := completion.Input
	for _, sub := range completion.Command.Commands {
		if sub.Hidden {
			continue
		}
		if strings.HasPrefix(sub.Name, prefix) {
			completion.Suggest(sub.Name, sub.Usage)
		} else if prefix != "" {
			// Prefer the canonical name; aliases only appear for a prefix it does not match.
			for _, alias := range sub.Aliases {
				if strings.HasPrefix(alias, prefix) {
					completion.Suggest(alias, sub.Usage)
				}
			}
		}
	}
}

// fixupOutput adds the input value when Bash would lose the entered text.
func (s *Completion) fixupOutput() {
	if s.Shell != shellBash {
		return
	}
	input := strings.TrimPrefix(s.AddPrefix+s.Input, s.CutPrefix)
	// Prevent Readline from inserting a shared prefix that loses the typed text.
	// FIXME: Compute the candidates' common prefix independently of suggestion
	// order. For Input "x", candidates "a:x", "a:y" add the preservation marker,
	// but reversing them omits it. Sorting Output afterward cannot fix this.
	common := input
	first := true
	for _, value := range s.Output {
		suggest, ok := strings.CutPrefix(s.AddPrefix+value.Text, s.CutPrefix)
		if !ok {
			continue
		}
		if first {
			first = false
			if pos := strings.Index(suggest, common); pos > 0 {
				common = suggest[:pos+len(common)]
				continue
			}
		}
		for !strings.HasPrefix(suggest, common) {
			common = common[:len(common)-1]
		}
	}
	if common != "" && !strings.Contains(common, input) {
		s.Debug("completion: preserve Bash input=%q: common_prefix=%q\n", input, common)
		s.Suggest(s.Input, "")
	}
}

func (s *Completion) enterCommand(command *Command) {
	s.Command = command
	s.Commands = append(s.Commands, command)
	s.collectFlags()
	s.StopFlags = false
	s.Debug("completion: enter command=%q flags=%d\n", command.Name, len(s.Flags))
}

func (s *Completion) flagsEnabled() bool {
	return !s.StopFlags && !s.Command.SkipFlagParsing && (s.Command.StopOnNthArg == nil || len(s.Positional) < *s.Command.StopOnNthArg)
}

// parseWords walks only the words before the cursor. It identifies the current word's
// handler without setting flags, parsing values, or running hooks.
func (s *Completion) parseWords() {
	// FIXME: Honor Command.DefaultCommand when resolving flags and arguments, or
	// document that completion does not support the normal parser's default routing.
	for _, word := range s.Words[:len(s.Words)-1] {
		s.Debug("completion: parse word=%q\n", word)
		if s.Pending != nil {
			// A required flag value takes precedence over delimiters and subcommands.
			s.Debug("completion: consume value=%q for flag=%q\n", word, s.Pending.Names())
			s.Pending = nil
			continue
		}
		if s.flagsEnabled() {
			if word == "--" {
				// FIXME: Continue positional completion after the flag delimiter, or
				// document that suppressing all suggestions here is intentional.
				s.Debug("completion: stop at flag delimiter\n")
				return // Remaining words are pass-through arguments.
			}
			if isFlagWord(word) {
				// Ignore unknown flags so incomplete input can still reach a handler.
				flag, _, hasValue := s.flag(word)
				s.Debug("completion: flag word=%q known=%t inline_value=%t\n", word, flag != nil, hasValue)
				if flag != nil && flagTakesValue(flag) && !hasValue {
					s.Pending = flag
				}
				continue
			}
			s.StopFlags = strings.HasPrefix(word, "-")
		}
		if len(s.Positional) == 0 {
			if sub := s.Command.Command(word); sub != nil {
				s.enterCommand(sub)
				continue
			}
		}
		s.Positional = append(s.Positional, word)
	}

	if s.Pending != nil {
		if completor, ok := s.Pending.(Completor); ok {
			s.Complete = completor.Complete
		}
		s.Debug("completion: handler=flag-value flag=%q supported=%t\n", s.Pending.Names(), s.Complete != nil)
		return
	}
	if s.flagsEnabled() && (s.Input == "-" || isFlagWord(s.Input)) {
		flag, value, hasValue := s.flag(s.Input)
		if hasValue {
			if completor, ok := flag.(Completor); ok {
				s.Complete = completor.Complete
			}
			s.AddPrefix = s.Input[:len(s.Input)-len(value)]
			if after, ok := strings.CutPrefix(s.AddPrefix, s.CutPrefix); ok {
				s.AddPrefix, s.CutPrefix = after, ""
			}
			s.Input = value
			s.Debug("completion: handler=inline-flag-value input=%q supported=%t\n", s.Input, s.Complete != nil)
			return
		}
		s.Complete = completeFlagNames
		s.Debug("completion: handler=flag-names input=%q\n", s.Input)
		return
	}
	for index, argument := range s.Command.Arguments {
		count := argumentMax(argument)
		if count == -1 || len(s.Positional) < count {
			if completor, ok := argument.(Completor); ok {
				s.Complete = completor.Complete
				s.Arguments = append(slices.Clone(s.Positional), s.Input)
			}
			s.Debug("completion: handler=argument index=%d type=%T arguments=%q supported=%t\n", index, argument, s.Arguments, s.Complete != nil)
			return
		}
		if count <= 0 {
			s.Debug("completion: argument index=%d has no remaining values\n", index)
			return
		}
		// Skip completed declarations so Arguments is local to the selected argument.
		s.Positional = s.Positional[count:]
	}
	if len(s.Command.Arguments) == 0 && len(s.Positional) == 0 {
		s.Complete = completeCommands
		s.Debug("completion: handler=commands input=%q\n", s.Input)
	}
}

// isFlagWord checks the first rune after '-' so negative numbers remain positional.
func isFlagWord(word string) bool {
	if !strings.HasPrefix(word, "-") {
		return false
	}
	for _, letter := range word[1:] {
		return letter == '-' || unicode.IsLetter(letter)
	}
	return false
}

func (s *Completion) flag(word string) (flag Flag, value string, hasValue bool) {
	name, value, hasValue := strings.Cut(strings.TrimLeft(word, "-"), "=")
	lookup := func(name string) Flag {
		for _, flag := range s.Flags {
			if slices.Contains(flag.Names(), name) {
				return flag
			}
		}
		return nil
	}
	flag = lookup(name)
	if flag == nil && s.shortOptionsEnabled() && !strings.HasPrefix(word, "--") {
		// Exact names win over short option groups; only the last option may need a value.
		for i, letter := range name {
			flag = lookup(string(letter))
			if flag == nil || flagTakesValue(flag) && i+len(string(letter)) < len(name) {
				return nil, value, hasValue
			}
		}
	}
	return flag, value, hasValue
}

func flagTakesValue(flag Flag) bool {
	// Optional-value wrappers override IsBoolFlag but may inherit TakesValue
	// from a string flag. Match urfave's parser by honoring that override first.
	if f, ok := flag.(interface{ IsBoolFlag() bool }); ok && f.IsBoolFlag() {
		return false
	}
	if f, ok := flag.(interface{ TakesValue() bool }); ok {
		return f.TakesValue()
	}
	return true
}

func (s *Completion) shortOptionsEnabled() bool {
	return slices.ContainsFunc(s.Commands, func(c *Command) bool { return c.UseShortOptionHandling })
}

// collectFlags reads definitions without applying flags or initializing commands.
// Walking upward gives child flags precedence; an alias collision hides the entire
// ancestor flag, matching urfave's persistent flag rules.
func (s *Completion) collectFlags() {
	s.Flags = nil
	seen := map[string]bool{}
	hideHelp := slices.ContainsFunc(s.Commands, func(c *Command) bool { return c.HideHelp })
	for i, command := range slices.Backward(s.Commands) {
		own := slices.Clone(command.Flags)
		for _, group := range command.MutuallyExclusiveFlags {
			for _, path := range group.Flags {
				own = append(own, path...)
			}
		}
		if i == len(s.Commands)-1 && !hideHelp && cli.HelpFlag != nil {
			own = append(own, cli.HelpFlag)
		}
		for _, flag := range own {
			if i < len(s.Commands)-1 {
				if f, ok := flag.(cli.LocalFlag); !ok || f.IsLocal() {
					continue
				}
			}
			names := flag.Names()
			if slices.ContainsFunc(names, func(name string) bool { return seen[name] }) {
				continue
			}
			s.Flags = append(s.Flags, flag)
			for _, name := range names {
				seen[name] = true
			}
		}
	}
}

func argumentMax(argument Argument) int {
	// Custom arguments can report their capacity without exposing a Max field.
	// A negative capacity of -1 means all remaining words belong to the argument.
	if argument, ok := argument.(interface{ MaxArgs() int }); ok {
		return argument.MaxArgs()
	}
	// cli.Argument has no arity method. Read the public Max configuration shared
	// by its variadic implementations; other arguments consume one word.
	value := reflect.Indirect(reflect.ValueOf(argument))
	if value.Kind() == reflect.Struct {
		if count := value.FieldByName("Max"); count.IsValid() && count.Kind() == reflect.Int {
			return int(count.Int())
		}
	}
	return 1
}

// normalizeWords rejoins Bash's --flag, =, value tokens into one logical word.
// CutPrefix records the part outside the cursor's replacement span so Reply can
// remove it after AddPrefix restores the flag name.
//
// A trailing colon token stays in the parser input but is outside Bash's
// replacement span. For example, server: arrives as ["server", ":"]. Keeping
// both words lets JSON completion suggest ":{"; setting CutPrefix to ":" makes
// Reply send only "{" to Bash, which already preserves the typed colon.
func (s *Completion) normalizeWords() {
	if s.Shell != shellBash {
		return
	}
	words := s.Words
	var args []string
	// FIXME: Reconstruct complete flag values and replacement spans across repeated
	// '=' word breaks. Bash splits --output=key=v into --output, =, key, =, v;
	// joining only the first triplet loses flag context and treats v as positional.
	for i := 0; i < len(words); i++ {
		word := words[i]
		if strings.HasPrefix(word, "-") && i+1 < len(words) && words[i+1] == "=" {
			prefix := word + "="
			word = prefix
			i++
			if i+1 < len(words) {
				i++
				word += words[i]
			}
			if i == len(words)-1 {
				s.CutPrefix = prefix
			}
		}
		args = append(args, word)
	}
	if len(words) > 0 {
		if last := words[len(words)-1]; last != "" && strings.Trim(last, ":") == "" {
			s.CutPrefix += last
		}
	}
	s.Words = args
}

// generateCompletionScript returns a path relative to XDG_DATA_HOME and its contents.
// command is the completed executable; handler is the executable serving requests.
func generateCompletionScript(shell, command, handler string, debug bool) (scriptPath string, script string, err error) {
	// FIXME: Validate --alias as a basename and escape command and handler for each
	// shell, including generated function identifiers. An alias like ../../../escaped
	// escapes the installation directory; shell metacharacters also enter the source.
	debugFlag := ""
	if debug {
		debugFlag = " --debug"
	}
	switch shell {
	case shellBash:
		scriptPath = fmt.Sprintf("bash-completion/completions/%s", command)
		script = fmt.Sprintf(`__%s_complete_bash() {
  mapfile -t COMPREPLY < <("%[2]s" completion%[3]s --complete bash -- "${COMP_WORDS[@]:1:COMP_CWORD}")
}
complete -o bashdefault -o default -F __%[1]s_complete_bash %[1]s
`, command, handler, debugFlag)

	case shellFish:
		scriptPath = fmt.Sprintf("fish/vendor_completions.d/%s.fish", command)
		script = fmt.Sprintf(`function __fish_%[1]s_complete
  set -l args (commandline -opc) (commandline -ct)
  set -e args[1]
  set -g __fish_%[1]s_completions ("%[2]s" completion%[3]s --complete fish -- $args)
  set -q __fish_%[1]s_completions[1]
end
complete -f -n '__fish_%[1]s_complete' -a '$__fish_%[1]s_completions' -c "%[1]s"
`, command, handler, debugFlag)

	case shellZsh:
		scriptPath = fmt.Sprintf("zsh/site-functions/_%s", command)
		script = fmt.Sprintf(`#compdef %[1]s
_%[1]s() {
  local -a completions
  completions=(${(f)"$("%[2]s" completion%[3]s --complete zsh -- "${words[@]:1:$((CURRENT-1))}")"})
  if (( ${#completions[@]} )); then
    _describe 'completions' completions
  else
    _default
  fi
}
compdef _%[1]s %[1]s
if [ "$funcstack[1]" = "_%[1]s" ]; then
  _%[1]s
fi
`, command, handler, debugFlag)

	case "":
		return "", "", fmt.Errorf("shell is not specified")
	default:
		return "", "", fmt.Errorf("shell %q is not supported yet, supported: bash, fish, zsh", shell)
	}

	return scriptPath, script, nil
}

func doCompletionScript(writer io.Writer, shell, command, handler string, install, debug bool) error {
	scriptPath, script, err := generateCompletionScript(shell, command, handler, debug)
	if err != nil {
		return err
	}
	if install {
		dataHomeDir := os.Getenv("XDG_DATA_HOME")
		if dataHomeDir == "" {
			userHomeDir, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dataHomeDir = filepath.Join(userHomeDir, ".local", "share")
		}
		scriptPath = filepath.Join(dataHomeDir, scriptPath)
		if _, err := fmt.Fprintf(writer, "Installing completion script: %v\n", scriptPath); err != nil {
			return err
		}
		//nolint:gosec //0o755
		if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
			return fmt.Errorf("failed to create completion script directory: %w", err)
		}
		//nolint:gosec //0o644
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			return fmt.Errorf("failed to install completion script: %w", err)
		}
	} else if _, err := writer.Write([]byte(script)); err != nil {
		return err
	}
	return nil
}

// CompletionCommand serves completion requests or prints/installs a shell script.
func CompletionCommand() *Command {
	// FIXME: Dispatch --complete before normal root hooks and required-flag checks.
	// Action runs too late: a required root --token blocks completion and can print
	// help into its output, while application hooks still run for every request.
	var complete bool
	var debug bool
	var install bool
	var shell string
	var alias string
	var completeArgs []string
	return &Command{
		Name:      "completion",
		Usage:     "generate shell completion",
		ArgsUsage: "[options]... [shell] [-- complete-args]...",
		Flags: []Flag{
			&BoolFlag{
				Name:        "complete",
				Destination: &complete,
				Usage:       "Generate completion for arguments.",
			},
			&BoolFlag{
				Name:        "debug",
				Destination: &debug,
				Usage:       "Print completion debug messages and enable them in generated scripts.",
			},
			&BoolFlag{
				Name:        "install",
				Destination: &install,
				Usage:       "Install shell completion script into $XDG_DATA_HOME, ~/.local/share/...",
			},
			&StringFlag{
				Name:        "alias",
				Destination: &alias,
				Usage:       "Generate completion script for command alias",
			},
		},
		Arguments: []Argument{
			&StringArg{
				Name:        "shell",
				Destination: &shell,
				Suggestions: []Suggestion{{Text: shellBash}, {Text: shellFish}, {Text: shellZsh}},
			},
			&StringArgs{Name: "complete-args", Destination: &completeArgs, Max: -1},
		},
		Action: func(ctx context.Context, c *Command) error {
			if shell == "" {
				shell = filepath.Base(os.Getenv("SHELL"))
			}
			if complete {
				var debugWriter io.Writer
				if debug {
					debugWriter = c.ErrWriter
				}
				completion := MakeCompletion(ctx, c.Root(), shell, completeArgs, debugWriter)
				return completion.Reply(c.Writer)
			}
			handler := c.Root().Name
			if alias == "" {
				alias = handler
			}
			return doCompletionScript(c.Writer, shell, alias, handler, install, debug)
		},
	}
}
