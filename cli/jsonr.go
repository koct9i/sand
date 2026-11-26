package cli

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"

	"github.com/koct9i/sand/jsonr"
)

// RelaxedJSONArguments parses successive arguments as lines of one relaxed JSON object.
// Outer braces are optional. T describes the object; JSON field names and help
// tags supply completion candidates and descriptions, including nested fields.
// Value completion suggests literals and container openers using their Go types.
type RelaxedJSONArguments[T any] struct {
	Name        string         `json:"name"`
	Value       T              `json:"value"`
	Destination *T             `json:"-"`
	UsageText   string         `json:"usageText"`
	Options     []json.Options `json:"-"`

	value *T
}

var (
	_ Argument  = (*RelaxedJSONArguments[struct{}])(nil)
	_ Completor = (*RelaxedJSONArguments[struct{}])(nil)
)

func (a *RelaxedJSONArguments[T]) HasName(name string) bool {
	return name == a.Name
}

// MaxArgs reports that the JSON document consumes all remaining arguments.
func (a *RelaxedJSONArguments[T]) MaxArgs() int {
	return -1
}

func (a *RelaxedJSONArguments[T]) Usage() string {
	return (&StringArgs{Name: a.Name, UsageText: a.UsageText, Min: 1, Max: -1}).Usage()
}

func (a *RelaxedJSONArguments[T]) Get() any {
	if a.value != nil {
		return *a.value
	}
	return a.Value
}

func (a *RelaxedJSONArguments[T]) Parse(args []string) ([]string, error) {
	count := len(args)

	// FIXME: Decode into an isolated copy of Value. This shallow copy shares maps,
	// slices, and pointers with defaults and possibly Destination, so a failed
	// decode can mutate them and Parse(nil) cannot restore the original defaults.
	value := a.Value
	if count > 0 {
		input := strings.Join(args[:count], "\n")
		reader := jsonr.NewRelaxedReader(strings.NewReader(input))
		reader.Wrap('{')
		if err := reader.SourceError(json.UnmarshalRead(reader, &value, a.Options...)); err != nil {
			return args, fmt.Errorf("invalid relaxed JSON for argument %s: %w", a.Name, err)
		}
	}
	a.value = &value
	if a.Destination != nil {
		*a.Destination = value
	}
	return args[count:], nil
}

// Complete replaces the current shell argument using the preceding JSON words as
// context. It omits keys already present in the current object and accepts
// incomplete input without decoding it into Destination.
func (a *RelaxedJSONArguments[T]) Complete(_ context.Context, c *Completion) {
	args := c.Arguments
	if len(args) == 0 {
		args = []string{c.Input}
	}
	word := c.Input
	input := strings.Join(args, "\n")
	reader := jsonr.NewRelaxedReader(strings.NewReader(input))
	reader.Wrap('{')
	comp := jsonr.MakeCompletion(reader, a.Options...)
	c.Debug("completion: JSON token=%q pointer=%q start=%d value=%t closer=%q\n", comp.Token, comp.Pointer, comp.Start, comp.Value, comp.Closer)
	if comp.Start < 0 {
		c.Debug("completion: skip JSON completion inside a comment\n")
		return // The cursor is inside a comment.
	}
	// Translate the token's byte offset from the full input to the current word.
	// For "name:demo\nserver:{ho": wordStart=10, comp.Start=18, tokenOffset=8.
	wordStart := len(input) - len(word)
	tokenOffset := comp.Start - wordStart
	if tokenOffset < 0 {
		c.Debug("completion: skip JSON token starting in an earlier word\n")
		return // The token starts in an earlier shell word.
	}
	prefix := word[:tokenOffset]
	completions := map[string]string{}
	comp.Complete(reflect.TypeFor[T](), completions)
	for replacement, help := range completions {
		c.Suggest(prefix+replacement, help)
	}
}
