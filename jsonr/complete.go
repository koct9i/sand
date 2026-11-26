package jsonr

import (
	"bytes"
	"encoding"
	"encoding/json/jsontext"
	"errors"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Completion holds the token and parser state at the end of relaxed JSON input.
type Completion struct {
	// Token is the active token's text, unquoted for strings. Separators and
	// trailing trivia clear it; unfinished escapes retain their encoded text.
	Token string
	// Pointer is the JSON Pointer to the value being completed. While completing
	// an object name, its final component is empty until a colon is entered.
	Pointer string
	// Closer contains the remaining closing quote and containers, innermost first,
	// excluding the implicit Wrap. It can be nonempty even when closing is invalid.
	Closer string
	// Start is the token's source byte offset, including its opening quote.
	// With no active token, it is the source bytes read; -1 means inside a comment.
	Start int
	// Value selects value suggestions rather than object names or a finished document.
	Value bool
	// Keys contains unquoted names already read in the current object, excluding
	// the active name being completed. It is nil outside an object.
	Keys []string
	// Wrap is the implicit outer container ('{' or '['), or zero when disabled.
	Wrap byte
	// canClose reports whether the cursor is at a token boundary that permits Closer.
	canClose bool
	// finished marks a completed object or array at the cursor so the shell can finish the word.
	finished bool
	// colonPointer identifies a completed object name awaiting its colon after trivia.
	colonPointer string
}

// MakeCompletion reads input and returns the token at the end of relaxed JSON input,
// its completion position as a JSON Pointer, the closing quotes and containers
// still open, and the token's byte offset in the original input, including any
// opening quote. Value reports whether the cursor is completing a value rather
// than an object name or an already completed document.
// Keys contains the unquoted names already read in the current object, excluding
// the active name being completed. It is nil outside an object.
// A *RelaxedReader input is used directly, preserving its wrapping configuration.
// Wrap reports its implicit container, or zero when wrapping is disabled.
// Implicit containers stay open at EOF and are excluded from Closer.
// When there is no active token, the offset is the number of source bytes read.
// An offset of -1 means input ends inside a comment.
// String tokens are unquoted. A separator or trailing whitespace clears token;
// arrays then point to the next element, and object names use an empty final
// pointer component until a colon is entered. Incomplete strings with unfinished
// escapes retain their encoded content. Parsing stops at the first syntax error.
func MakeCompletion(in *RelaxedReader, opts ...jsontext.Options) Completion {
	var c Completion
	in.openWrap = true
	d := jsontext.NewDecoder(in, append([]jsontext.Options{jsontext.AllowDuplicateNames(true)}, opts...)...)
	var last jsontext.Kind
	var text string
	var err error
	var keyStack [][]string
	for {
		var t jsontext.Token
		t, err = d.ReadToken()
		if err != nil {
			break
		}
		depth := d.StackDepth()
		switch t.Kind() {
		case '{', '[':
			keyStack = append(keyStack, nil)
		case '}', ']':
			keyStack = keyStack[:depth]
		case '"':
			if kind, count := d.StackIndex(depth); kind == '{' && count%2 == 1 {
				keyStack[depth-1] = append(keyStack[depth-1], t.String())
			}
		}
		last, text = t.Kind(), t.String()
	}

	c.Wrap = in.wrap
	c.Start = int(in.inOffset)
	tail := d.UnreadBuffer()
	atToken := in.inOffset > in.triviaEnd
	active := false
	switch last {
	case '"', '0', 'n', 't', 'f':
		active = len(tail) == 0 && atToken
		if active {
			c.Token = text
			c.Start = int(in.lastStart)
		}
	}
	rest := bytes.TrimLeft(tail, " \t\r\n")
	colon := len(rest) > 0 && rest[0] == ':'
	if len(rest) > 0 && (rest[0] == ':' || rest[0] == ',') {
		rest = bytes.TrimLeft(rest[1:], " \t\r\n")
	}
	if atToken && errors.Is(err, io.ErrUnexpectedEOF) && len(rest) > 0 {
		switch {
		case rest[0] == '"':
			c.Start = int(in.lastStart)
			c.Token = string(rest[1:])
			if decoded, err := jsontext.AppendUnquote(nil, string(rest)+`"`); err == nil {
				c.Token = string(decoded)
			}
			c.Closer = string(in.last)
		case rest[0] == '-' || rest[0] >= '0' && rest[0] <= '9':
			c.Start = int(in.lastStart)
			c.Token = string(rest)
		}
	}

	depth := d.StackDepth()
	path := d.StackPointer()
	kind, count := d.StackIndex(depth)
	if kind == '{' && count%2 == 1 && !active && len(tail) == 0 &&
		errors.Is(in.err, io.EOF) && !in.inComment && errors.Is(err, io.ErrUnexpectedEOF) {
		c.colonPointer = string(path)
	}
	// Closing the innermost container completes the value in every parent.
	// Only offer closers at a token boundary, so a value prefix such as "t"
	// completes to "true" without competing with "t}". Objects need paired
	// names and values before closing.
	c.canClose = c.Start == int(in.inOffset) && len(rest) == 0 && (kind != '{' || count%2 == 0)
	c.canClose = c.canClose && errors.Is(in.err, io.EOF) && !in.inComment &&
		(errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
	if c.Wrap != 0 && depth > 0 {
		outer, _ := d.StackIndex(1)
		c.canClose = c.canClose && outer == jsontext.Kind(c.Wrap)
	}
	base := path
	if count > 0 {
		base = path.Parent()
	}
	switch kind {
	case '{':
		c.Keys = keyStack[depth-1]
		if active && count%2 == 1 {
			c.Keys = c.Keys[:len(c.Keys)-1]
		}
		c.Value = count%2 == 1 && colon || count%2 == 0 && active
		if c.Value {
			c.Pointer = string(path)
		} else {
			c.Pointer = string(base.AppendToken(""))
		}
	case '[':
		c.Value = true
		if active {
			count--
		}
		c.Pointer = string(base.AppendToken(strconv.FormatInt(count, 10)))
	default:
		c.Value = count == 0 || active
	}
	closing := make([]byte, 0, len(c.Closer)+depth)
	closing = append(closing, c.Closer...)
	for i := depth; i > 0; i-- {
		kind, _ := d.StackIndex(i)
		if kind == '{' {
			closing = append(closing, '}')
		} else {
			closing = append(closing, ']')
		}
	}
	c.Closer = string(closing)
	switch c.Wrap {
	case '{':
		c.Closer = strings.TrimSuffix(c.Closer, "}")
	case '[':
		c.Closer = strings.TrimSuffix(c.Closer, "]")
	}
	if in.inComment {
		c.Start = -1
	}
	c.finished = errors.Is(in.err, io.EOF) && atToken && len(tail) == 0 && (last == '}' || last == ']')
	return c
}

// Complete adds matching relaxed JSON replacements and descriptions to completions.
// s may be a value, a pointer to one (including nil), or its reflect.Type.
// Value selects between value suggestions and object names.
// Field names respect JSON tags and embedded structs, exclude Keys, and include
// a following colon. String-keyed maps suggest key placeholders or the entered
// name followed by a colon. Struct fields use help tags as descriptions; other suggestions
// use their JSON Pointer. A completed object or array returns an empty replacement
// so shells can finish the current word, alongside any remaining outer closers.
// After a completed object name and trailing trivia, only its missing colon is suggested.
// Closing suggestions use "close" as the description and appear only at a token
// boundary or after a completed container, never while completing a scalar or
// string. MakeCompletion records whether closing is possible during its initial
// parse; Complete does not parse the input again. Comments produce no suggestions.
func (c Completion) Complete(s any, completions map[string]string) {
	if completions == nil || c.Start < 0 {
		return
	}
	if c.colonPointer != "" {
		completions[":"] = c.colonPointer
		return
	}
	if c.canClose && c.Closer != "" {
		completions[c.Closer] = "close"
	}
	if c.finished {
		completions[""] = ""
		return
	}
	root := completionRoot(s)
	if c.Value {
		c.completeValue(root, completions)
	} else if c.Pointer != "" {
		c.completeFields(root, completions)
	}
}

func (c Completion) completeFields(root reflect.Type, completions map[string]string) {
	p := jsontext.Pointer(c.Pointer)
	if c.Pointer != "" {
		if !p.IsValid() || !strings.HasSuffix(c.Pointer, "/") {
			return
		}
		p = p.Parent()
	}
	t := completionType(completionAt(root, p))
	if t == nil {
		return
	}
	suggest := func(name, note string) {
		if !strings.HasPrefix(name, c.Token) || slices.Contains(c.Keys, name) {
			return
		}
		if encoded, err := Marshal(name); err == nil {
			completions[string(encoded)+":"] = note
		}
	}
	switch t.Kind() {
	case reflect.Struct:
		for name, field := range completionFields(t) {
			if !field.ambiguous {
				suggest(name, field.help)
			}
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return
		}
		names := []string{c.Token}
		if c.Token == "" {
			names = []string{"_key_"}
		}
		for _, name := range names {
			suggest(name, string(p.AppendToken(name)))
		}
	}
}

func (c Completion) completeValue(root reflect.Type, completions map[string]string) {
	t := completionAt(root, jsontext.Pointer(c.Pointer))
	if t == nil {
		return
	}
	nullable := t.Kind() == reflect.Pointer
	t = completionType(t)
	nullable = nullable || t.Kind() == reflect.Map || t.Kind() == reflect.Slice || t.Kind() == reflect.Interface
	var candidates []string
	switch {
	case reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()):
		candidates = []string{`"`}
	default:
		switch t.Kind() {
		case reflect.Struct:
			candidates = []string{"{}"}
			fields := map[string]string{}
			Completion{}.completeFields(t, fields)
			for name, help := range fields {
				candidate := "{" + name
				if strings.HasPrefix(candidate, c.Token) {
					completions[candidate] = help
				}
			}
		case reflect.Map:
			candidates = []string{"{}"}
		case reflect.Array, reflect.Slice:
			if t.Elem().Kind() == reflect.Uint8 {
				if t.Kind() == reflect.Array && t.Len() != 0 {
					candidates = []string{`"`}
				} else {
					candidates = []string{`""`}
				}
			} else {
				candidates = []string{"["}
			}
		case reflect.Bool:
			candidates = []string{jsontext.True.String(), jsontext.False.String()}
		case reflect.String:
			if c.Token == "" {
				candidates = []string{"", "_str_"}
			} else {
				candidates = []string{c.Token}
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64:
			if c.Token == "" {
				candidates = []string{"", "0"}
			} else {
				candidates = []string{c.Token}
			}
		case reflect.Interface:
			if t.NumMethod() == 0 {
				candidates = []string{"{}", "[", jsontext.True.String(), jsontext.False.String(), `""`, "0"}
			}
		default:
			return
		}
	}
	if nullable {
		candidates = append(candidates, jsontext.Null.String())
	}
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, c.Token) {
			completions[candidate] = c.Pointer
		}
	}
}

func completionRoot(s any) reflect.Type {
	if t, ok := s.(reflect.Type); ok {
		return t
	}
	return reflect.TypeOf(s)
}

func completionAt(t reflect.Type, pointer jsontext.Pointer) reflect.Type {
	if !pointer.IsValid() {
		return nil
	}
	for name := range pointer.Tokens() {
		t = completionType(t)
		if t == nil {
			return nil
		}
		switch t.Kind() {
		case reflect.Struct:
			field, ok := completionFields(t)[name]
			if !ok || field.ambiguous {
				return nil
			}
			t = field.typ
		case reflect.Array, reflect.Slice:
			i, err := strconv.ParseUint(name, 10, 64)
			if err != nil || strconv.FormatUint(i, 10) != name || t.Kind() == reflect.Array && i >= uint64(t.Len()) {
				return nil
			}
			t = t.Elem()
		case reflect.Map:
			t = t.Elem()
		default:
			return nil
		}
	}
	return t
}

func completionType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

type completionField struct {
	typ       reflect.Type
	help      string
	depth     int
	tagged    bool
	ambiguous bool
}

func completionFields(t reflect.Type) map[string]completionField {
	fields := make(map[string]completionField)
	visiting := make(map[reflect.Type]bool)
	var visit func(reflect.Type, int)
	visit = func(t reflect.Type, depth int) {
		if visiting[t] {
			return
		}
		visiting[t] = true
		defer delete(visiting, t)
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			name, tagged, embedded, ok := completionFieldName(f)
			if !ok {
				continue
			}
			if embedded {
				if ft := completionType(f.Type); ft.Kind() == reflect.Struct {
					visit(ft, depth+1)
				}
				continue
			}
			field := completionField{typ: f.Type, help: f.Tag.Get("help"), depth: depth, tagged: tagged}
			old, exists := fields[name]
			switch {
			case !exists || depth < old.depth || depth == old.depth && tagged && !old.tagged:
				fields[name] = field
			case depth == old.depth && tagged == old.tagged:
				old.ambiguous = true
				fields[name] = old
			}
		}
	}
	visit(t, 0)
	return fields
}

func completionFieldName(f reflect.StructField) (name string, tagged, embedded, ok bool) {
	tag := f.Tag.Get("json")
	options := ""
	if strings.HasPrefix(tag, "'") {
		end := 1
		for end < len(tag) {
			c := tag[end]
			end++
			if c == '\\' {
				end++
			} else if c == '\'' {
				break
			}
		}
		if end > len(tag) || Unmarshal([]byte(tag[:end]), &name) != nil {
			return "", false, false, false
		}
		options = strings.TrimPrefix(tag[end:], ",")
		tagged = true
	} else {
		name, options, _ = strings.Cut(tag, ",")
		tagged = name != ""
		if !tagged {
			name = f.Name
		}
	}
	embedded = f.Anonymous && !tagged
	for option := range strings.SplitSeq(options, ",") {
		if option == "embed" {
			embedded = true
		}
	}
	return name, tagged, embedded, true
}
