package jsonr

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// GetLastToken returns the token at the end of relaxed JSON input, its completion
// position as a JSON Pointer, and the closing quotes and containers still open.
// String tokens are unquoted. A separator or trailing whitespace clears token;
// arrays then point to the next element, and object names use an empty final
// pointer component until a colon is entered. Incomplete strings with unfinished
// escapes retain their encoded content. Parsing stops at the first syntax error.
func GetLastToken(in []byte) (token, pointer, closer string) {
	src := NewRelaxedReader(bytes.NewReader(in))
	d := jsontext.NewDecoder(src, jsontext.AllowDuplicateNames(true))
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

	tail := d.UnreadBuffer()
	active := false
	switch last {
	case '"', '0', 'n', 't', 'f':
		active = len(tail) == 0
		if active {
			token = text
		}
	}
	rest := bytes.TrimLeft(tail, " \t\r\n")
	colon := len(rest) > 0 && rest[0] == ':'
	if len(rest) > 0 && (rest[0] == ':' || rest[0] == ',') {
		rest = bytes.TrimLeft(rest[1:], " \t\r\n")
	}
	if errors.Is(err, io.ErrUnexpectedEOF) && len(rest) > 0 {
		switch {
		case rest[0] == '"':
			token = string(rest[1:])
			if decoded, err := jsontext.AppendUnquote(nil, string(rest)+`"`); err == nil {
				token = string(decoded)
			}
			closer = string(src.last)
		case rest[0] == '-' || rest[0] >= '0' && rest[0] <= '9':
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
		if count%2 == 1 && colon || count%2 == 0 && active {
			pointer = string(path)
		} else {
			pointer = string(base.AppendToken(""))
		}
	case '[':
		if active {
			count--
		}
		pointer = string(base.AppendToken(strconv.FormatInt(count, 10)))
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
	return token, pointer, closer
}

// CompletionFunc adds matching replacements and their descriptions to completions.
// pointer is relative to the handler's registration path; prefix is unquoted.
type CompletionFunc func(pointer, prefix string, completions map[string]string)

// Completor dispatches completion to the handler at the nearest ancestor pointer.
type Completor struct {
	Handlers map[string]CompletionFunc
}

// StructCompletion completes exported JSON field names using help tags as
// descriptions. s may be a struct, a pointer to one (including nil), or its
// reflect.Type. JSON names, ignored fields, and embedded structs are respected.
// An empty pointer completes the struct's own fields; otherwise the pointer must
// end in an empty component and may traverse structs, arrays, slices, and maps.
// Replacements are complete relaxed JSON strings, without a following colon.
func StructCompletion(s any) CompletionFunc {
	root, ok := s.(reflect.Type)
	if !ok {
		root = reflect.TypeOf(s)
	}
	return func(pointer, prefix string, completions map[string]string) {
		if completions == nil {
			return
		}
		t := root
		if pointer != "" {
			p := jsontext.Pointer(pointer)
			if !p.IsValid() || !strings.HasSuffix(pointer, "/") {
				return
			}
			for name := range p.Parent().Tokens() {
				t = completionType(t)
				if t == nil {
					return
				}
				switch t.Kind() {
				case reflect.Struct:
					field, ok := completionFields(t)[name]
					if !ok || field.ambiguous {
						return
					}
					t = field.typ
				case reflect.Array, reflect.Slice:
					i, err := strconv.ParseUint(name, 10, 64)
					if err != nil || strconv.FormatUint(i, 10) != name || t.Kind() == reflect.Array && i >= uint64(t.Len()) {
						return
					}
					t = t.Elem()
				case reflect.Map:
					t = t.Elem()
				default:
					return
				}
			}
		}
		t = completionType(t)
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

func (c *Completor) GetCompletions(in []byte) map[string]string {
	completions := map[string]string{}
	token, pointer, closer := GetLastToken(in)

	for p := jsontext.Pointer(pointer); ; p = p.Parent() {
		if handler := c.Handlers[string(p)]; handler != nil {
			handler(pointer[len(p):], token, completions)
			break
		}
		if p == "" {
			break
		}
	}

	if _, exists := completions[token+closer]; !exists {
		completions[token+closer] = ""
	}
	return completions
}
