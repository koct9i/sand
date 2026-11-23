package jsonr

import (
	"bytes"
	"encoding"
	"encoding/json/jsontext"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// GetLastToken reads input and returns the token at the end of relaxed JSON input,
// its completion position as a JSON Pointer, the closing quotes and containers
// still open, and the token's byte offset in the original input, including any
// opening quote. value reports whether the cursor is completing a value rather
// than an object name or an already completed document.
// When there is no active token, the offset is the number of source bytes read.
// An offset of -1 means input ends inside a comment.
// String tokens are unquoted. A separator or trailing whitespace clears token;
// arrays then point to the next element, and object names use an empty final
// pointer component until a colon is entered. Incomplete strings with unfinished
// escapes retain their encoded content. Parsing stops at the first syntax error.
func GetLastToken(input io.Reader) (token, pointer, closer string, start int, value bool) {
	relaxed := NewRelaxedReader(input)
	d := jsontext.NewDecoder(relaxed, jsontext.AllowDuplicateNames(true))
	var last jsontext.Kind
	var text string
	var err error
	for {
		var t jsontext.Token
		t, err = d.ReadToken()
		if err != nil {
			break
		}
		last, text = t.Kind(), t.String()
	}

	start = int(relaxed.inOffset)
	tail := d.UnreadBuffer()
	atToken := relaxed.inOffset > relaxed.triviaEnd
	active := false
	switch last {
	case '"', '0', 'n', 't', 'f':
		active = len(tail) == 0 && atToken
		if active {
			token = text
			start = int(relaxed.lastStart)
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
			start = int(relaxed.lastStart)
			token = string(rest[1:])
			if decoded, err := jsontext.AppendUnquote(nil, string(rest)+`"`); err == nil {
				token = string(decoded)
			}
			closer = string(relaxed.last)
		case rest[0] == '-' || rest[0] >= '0' && rest[0] <= '9':
			start = int(relaxed.lastStart)
			token = string(rest)
		}
	}

	depth := d.StackDepth()
	path := d.StackPointer()
	kind, count := d.StackIndex(depth)
	base := path
	if count > 0 {
		base = path.Parent()
	}
	switch kind {
	case '{':
		value = count%2 == 1 && colon || count%2 == 0 && active
		if value {
			pointer = string(path)
		} else {
			pointer = string(base.AppendToken(""))
		}
	case '[':
		value = true
		if active {
			count--
		}
		pointer = string(base.AppendToken(strconv.FormatInt(count, 10)))
	default:
		value = count == 0 || active
	}
	closing := make([]byte, 0, len(closer)+depth)
	closing = append(closing, closer...)
	for i := depth; i > 0; i-- {
		kind, _ := d.StackIndex(i)
		if kind == '{' {
			closing = append(closing, '}')
		} else {
			closing = append(closing, ']')
		}
	}
	closer = string(closing)
	if relaxed.inComment {
		start = -1
	}
	return token, pointer, closer, start, value
}

// CompletionFunc adds matching replacements and their descriptions to completions.
// pointer is the completion position as a JSON Pointer; prefix is unquoted.
type CompletionFunc func(pointer, prefix string, completions map[string]string)

// StructCompletion completes exported JSON field names using help tags as
// descriptions. s may be a struct, a pointer to one (including nil), or its
// reflect.Type. JSON names, ignored fields, and embedded structs are respected.
// An empty pointer completes the struct's own fields; otherwise the pointer must
// end in an empty component and may traverse structs, arrays, slices, and maps.
// Replacements are complete relaxed JSON strings, without a following colon.
func StructCompletion(s any) CompletionFunc {
	root := completionRoot(s)
	return func(pointer, prefix string, completions map[string]string) {
		if completions == nil {
			return
		}
		p := jsontext.Pointer(pointer)
		if pointer != "" {
			if !p.IsValid() || !strings.HasSuffix(pointer, "/") {
				return
			}
			p = p.Parent()
		}
		t := completionType(completionAt(root, p))
		if t == nil || t.Kind() != reflect.Struct {
			return
		}
		for name, field := range completionFields(t) {
			if field.ambiguous || !strings.HasPrefix(name, prefix) {
				continue
			}
			if encoded, err := Marshal(name); err == nil {
				completions[string(encoded)] = field.help
			}
		}
	}
}

// ValueCompletion completes values at a JSON Pointer using their Go type.
// It suggests container openers, boolean literals, string placeholders, zero
// numbers, and null for nullable types. Empty interfaces accept every JSON value kind.
// String values always suggest _STRING_ and an empty replacement, regardless of
// prefix, to prevent the shell from falling back to unrelated completions.
// Each suggestion's description is its JSON Pointer.
// Byte arrays and text-unmarshaled types use an opening quote when an empty
// string may not be a valid value. prefix is the unquoted token being replaced.
// s accepts the same value and reflect.Type forms as StructCompletion.
func ValueCompletion(s any) CompletionFunc {
	root := completionRoot(s)
	return func(pointer, prefix string, completions map[string]string) {
		if completions == nil {
			return
		}
		t := completionAt(root, jsontext.Pointer(pointer))
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
			case reflect.Struct, reflect.Map:
				candidates = []string{"{"}
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
				candidates = []string{"true", "false"}
			case reflect.String:
				if prefix == "" {
					candidates = []string{"_", "string"}
				} else {
					candidates = []string{prefix, prefix+","}
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
				reflect.Float32, reflect.Float64:
				candidates = []string{"0", "1"}
			case reflect.Interface:
				if t.NumMethod() == 0 {
					candidates = []string{"{", "[", "true", "false", `""`, "0"}
				}
			default:
				return
			}
		}
		if nullable {
			candidates = append(candidates, "null")
		}
		for _, candidate := range candidates {
			if strings.HasPrefix(candidate, prefix) {
				completions[candidate] = pointer
			}
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
