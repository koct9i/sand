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
// Like StringArgs, Max must be positive or -1 for unlimited arguments.
type RelaxedJSONArguments[T any] struct {
	Name        string         `json:"name"`
	Value       T              `json:"value"`
	Destination *T             `json:"-"`
	UsageText   string         `json:"usageText"`
	Min         int            `json:"minTimes"`
	Max         int            `json:"maxTimes"`
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

func (a *RelaxedJSONArguments[T]) Usage() string {
	return (&StringArgs{Name: a.Name, UsageText: a.UsageText, Min: a.Min, Max: a.Max}).Usage()
}

func (a *RelaxedJSONArguments[T]) Get() any {
	if a.value != nil {
		return *a.value
	}
	return a.Value
}

func (a *RelaxedJSONArguments[T]) Parse(args []string) ([]string, error) {
	if a.Max == 0 || a.Max < -1 || a.Min < 0 || a.Max != -1 && a.Min > a.Max {
		return args, fmt.Errorf("argument %s has invalid limits: min %d, max %d", a.Name, a.Min, a.Max)
	}
	count := len(args)
	if a.Max > 0 {
		count = min(count, a.Max)
	}
	if count < a.Min {
		return args, fmt.Errorf("insufficient arguments for %s: got %d, need %d", a.Name, count, a.Min)
	}

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
// context. It omits struct keys already present in the current object and accepts
// incomplete input without decoding it into Destination.
func (a *RelaxedJSONArguments[T]) Complete(_ context.Context, c *Completion) {
	args := c.Arguments
	if len(args) == 0 {
		args = []string{c.Input}
	}
	prefix := c.Input
	input := strings.Join(args, "\n")
	reader := jsonr.NewRelaxedReader(strings.NewReader(input))
	reader.Wrap('{')
	comp := jsonr.MakeCompletion(reader)
	if comp.Start < 0 {
		return // The cursor is inside a comment.
	}
	offset := comp.Start - (len(input) - len(prefix))
	if offset >= 0 {
		completions := map[string]string{}
		comp.Complete(reflect.TypeFor[T](), completions)
		for replacement, help := range completions {
			c.Suggest(prefix[:offset]+replacement, help)
		}
	}
	if comp.Closer != "" {
		var value T
		reader := jsonr.NewRelaxedReader(strings.NewReader(input + comp.Closer))
		reader.Wrap(comp.Wrap)
		if err := json.UnmarshalRead(reader, &value, a.Options...); err == nil {
			c.Suggest(prefix+comp.Closer, "close")
		}
	}
}
