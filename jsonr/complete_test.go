package jsonr

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMakeCompletion(t *testing.T) {
	for _, tt := range []struct{ input, token, pointer, closer string }{
		{``, "", "", ""},
		{` `, "", "", ""},
		{`{`, "", "/", "}"},
		{`{a`, "a", "/", "}"},
		{`{a:`, "", "/a", "}"},
		{`{a:[ `, "", "/a/0", "]}"},
		{`{a:[0`, "0", "/a/0", "]}"},
		{`{a:[0 `, "", "/a/1", "]}"},
		{`{a:[0,`, "", "/a/1", "]}"},
		{`{a:[0 bare`, "bare", "/a/1", "]}"},
		{`{a:[0 bare `, "", "/a/2", "]}"},
		{`{a:[0 {b`, "b", "/a/1/", "}]}"},
		{`{a:[0 {b:`, "", "/a/1/b", "}]}"},
		{`{a:[0 {b:true`, "true", "/a/1/b", "}]}"},
		{`{a:[0 {b:true `, "", "/a/1/", "}]}"},
		{`{a:[0 {b:true}`, "", "/a/2", "]}"},
		{`{a:[0 {b:true}]`, "", "/", "}"},
		{`{a:[0 {b:true}]}`, "", "", ""},
		{`{a:1 b`, "b", "/", "}"},
		{`{a:1 a`, "a", "/", "}"},
		{`{a:1 b:`, "", "/b", "}"},
		{`{a:1,`, "", "/", "}"},
		{`{'a/b~c':[1 `, "", "/a~1b~0c/1", "]}"},
		{`{'':`, "", "/", "}"},
		{`{a:bare`, "bare", "/a", "}"},
		{`{a:0`, "0", "/a", "}"},
		{`{a:'text'`, "text", "/a", "}"},
		{`{a:''`, "", "/a", "}"},
		{`[''`, "", "/0", "]"},
		{`{a:'unfinished`, "unfinished", "/a", "'}"},
		{`{a:'it\'s`, "it's", "/a", "'}"},
		{`{'na`, "na", "/", "'}"},
		{`{a:"it\"s`, `it"s`, "/a", `"}`},
		{`{a:"x\`, `x\`, "/a", `"}`},
		{`{a:"\u12`, `\u12`, "/a", `"}`},
		{`[1e`, "1e", "/0", "]"},
		{`[1e `, "", "/0", "]"},
		{`[1e/* done */`, "", "/0", "]"},
		{`[1 -`, "-", "/1", "]"},
		{`[1 - /* done */`, "", "/1", "]"},
		{`[1 ?`, "", "/1", "]"},
		{`'root`, "root", "", "'"},
		{`hello`, "hello", "", ""},
		{`42`, "42", "", ""},
		{`42 `, "", "", ""},
		{`true`, "true", "", ""},
		{`null`, "null", "", ""},
		{`# comment only`, "", "", ""},
		{`{a:/* comment */`, "", "/a", "}"},
		{`[1/* comment */`, "", "/1", "]"},
		{"[1 # comment\n", "", "/1", "]"},
		{`[1 2 3 4 5 6 7 8 9 10 `, "", "/10", "]"},
		{`{a:` + strings.Repeat("x", 8192), strings.Repeat("x", 8192), "/a", "}"},
		{`{a:'` + strings.Repeat("x", 8192), strings.Repeat("x", 8192), "/a", "'}"},
		{`[1` + strings.Repeat(" ", 8192), "", "/1", "]"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := MakeCompletion(NewRelaxedReader(strings.NewReader(tt.input)))
			if got.Token != tt.token || got.Pointer != tt.pointer || got.Closer != tt.closer {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", got.Token, got.Pointer, got.Closer, tt.token, tt.pointer, tt.closer)
			}
		})
	}
}

func TestMakeCompletionValuePosition(t *testing.T) {
	for _, tt := range []struct {
		input string
		value bool
	}{
		{``, true}, {`hello`, true}, {`42`, true}, {`42 `, false},
		{`{`, false}, {`{name`, false}, {`{name `, false},
		{`{name:`, true}, {`{name: /* done */`, true}, {`{name:'partial`, true},
		{`{name:null`, true}, {`{name:null `, false}, {`{name:{}`, false},
		{`{'':`, true}, {`{'':{`, false}, {`{map:{'':`, true},
		{`[`, true}, {`[1`, true}, {`[1 `, true}, {`[1,`, true},
		{`[1 {} `, true}, {`[1 {}]`, false}, {`{name:1}`, false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := MakeCompletion(NewRelaxedReader(strings.NewReader(tt.input)))
			if got.Value != tt.value {
				t.Fatalf("value position = %t, want %t", got.Value, tt.value)
			}
		})
	}
}

func TestMakeCompletionStart(t *testing.T) {
	for _, tt := range []struct{ input, source string }{
		{``, ""},
		{" \t\r\n", ""},
		{`{`, ""},
		{`{na`, "na"},
		{`{name:`, ""},
		{`{name:""`, `""`},
		{`{name:'it\'s`, `'it\'s`},
		{`{name:"\u0068o`, `"\u0068o`},
		{`{name:"\u0068o"`, `"\u0068o"`},
		{`{name:"x\`, `"x\`},
		{`{'two w`, `'two w`},
		{`{name:"x"en`, "en"},
		{`{name:'x'en`, "en"},
		{`{name:'x'/* done */en`, "en"},
		{`/* 日本語 */{na`, "na"},
		{`{a:[0`, "0"},
		{`{a:[0 `, ""},
		{`{a:[0/* done */`, ""},
		{`[1e `, ""},
		{`[1e/* done */`, ""},
		{`{a:[0,`, ""},
		{`[1 -`, "-"},
		{`[1/* done */2e-`, "2e-"},
		{`{a:/* done */`, ""},
		{"{a:1 # done\n", ""},
		{"{a:1 // done\r", ""},
		{`["# /* //"`, `"# /* //"`},
		{`{name:'x'}`, ""},
		{`{name:'` + strings.Repeat("x", 8192), `'` + strings.Repeat("x", 8192)},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := MakeCompletion(NewRelaxedReader(strings.NewReader(tt.input)))
			want := len(tt.input) - len(tt.source)
			if got.Start != want {
				t.Fatalf("token offset = %d, want %d for source spelling %q", got.Start, want, tt.source)
			}
		})
	}
	for _, input := range []string{
		`# comment`, `// comment`, `/* unfinished`, `/*/`,
		`{a:1 # comment`, `{a:1 // comment`, `{a:1 /* unfinished`,
		`{a:1, # comment`, `{a:1, // comment`, `{a:1, /* unfinished`,
	} {
		t.Run(input, func(t *testing.T) {
			got := MakeCompletion(NewRelaxedReader(strings.NewReader(input)))
			if got.Start != -1 {
				t.Fatalf("token offset inside a comment = %d, want -1", got.Start)
			}
		})
	}
}

func TestMakeCompletionKeys(t *testing.T) {
	for _, tt := range []struct {
		input string
		keys  []string
	}{
		{``, nil},
		{`{`, nil},
		{`{a`, nil},
		{`{"a"`, nil},
		{`{a:`, []string{"a"}},
		{`{a:1`, []string{"a"}},
		{`{a:1 a`, []string{"a"}},
		{`{a:1 b`, []string{"a"}},
		{`{a:1 b:`, []string{"a", "b"}},
		{`{a:1 b:{`, nil},
		{`{a:1 b:{c:2 `, []string{"c"}},
		{`{a:1 b:{c:2} `, []string{"a", "b"}},
		{`{a:1 b:[2 `, nil},
		{`{a:1 b:[{c:2 `, []string{"c"}},
		{`{a:1 b:[{c:2} {d:3 `, []string{"d"}},
		{`{a:1 b:[] `, []string{"a", "b"}},
		{`{"\u0061":1 'two words':2 n`, []string{"a", "two words"}},
		{`{'':1 n`, []string{""}},
		{`{a:1 /* b:2 */ n`, []string{"a"}},
		{"{a:1 // b:2\nn", []string{"a"}},
		{`{a:{x:1} a:{y`, nil},
		{`{a:{x:1} a:{y:2 `, []string{"y"}},
		{`{a:1}`, nil},
		{`[{} `, nil},
		{`[1 {b:2 `, []string{"b"}},
		{`{a:"unfinished`, []string{"a"}},
		{`{'unfinished`, nil},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got := MakeCompletion(NewRelaxedReader(strings.NewReader(tt.input)))
			if !slices.Equal(got.Keys, tt.keys) {
				t.Fatalf("keys = %q, want %q", got.Keys, tt.keys)
			}
		})
	}
}

func TestMakeCompletionWrap(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		wrap        byte
		want        Completion
	}{
		{"unwrapped", `{a:1 b`, 0, Completion{Token: "b", Pointer: "/", Closer: "}", Start: 5, Keys: []string{"a"}}},
		{"empty object", ``, '{', Completion{Pointer: "/", Wrap: '{'}},
		{"object prefix", `na`, '{', Completion{Token: "na", Pointer: "/", Wrap: '{'}},
		{"object value", `a:1`, '{', Completion{Token: "1", Pointer: "/a", Start: 2, Value: true, Keys: []string{"a"}, Wrap: '{'}},
		{"nested object", `a:{b:1 `, '{', Completion{Pointer: "/a/", Closer: "}", Start: 7, Keys: []string{"b"}, Wrap: '{'}},
		{"explicit object", `{a:1 b`, '{', Completion{Token: "b", Pointer: "/", Closer: "}", Start: 5, Keys: []string{"a"}}},
		{"comment before explicit object", `/* { */ {a:1 b`, '{', Completion{Token: "b", Pointer: "/", Closer: "}", Start: 13, Keys: []string{"a"}}},
		{"comment before implicit object", `/* { */ a:1 b`, '{', Completion{Token: "b", Pointer: "/", Start: 12, Keys: []string{"a"}, Wrap: '{'}},
		{"comment only", `/* done */`, '{', Completion{Pointer: "/", Start: 10, Wrap: '{'}},
		{"active comment", `# note`, '{', Completion{Pointer: "/", Start: -1, Wrap: '{'}},
		{"unfinished key", `'na`, '{', Completion{Token: "na", Pointer: "/", Closer: "'", Wrap: '{'}},
		{"completed object", `{}`, '{', Completion{Start: 2}},
		{"empty array", ``, '[', Completion{Pointer: "/0", Value: true, Wrap: '['}},
		{"array values", `1 2`, '[', Completion{Token: "2", Pointer: "/1", Start: 2, Value: true, Wrap: '['}},
		{"nested array", `[1`, '[', Completion{Token: "1", Pointer: "/0/0", Closer: "]", Start: 1, Value: true, Wrap: '['}},
		{"array wrapping stays unconditional", `[1]`, '[', Completion{Pointer: "/1", Start: 3, Value: true, Wrap: '['}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewRelaxedReader(strings.NewReader(tt.input))
			reader.Wrap(tt.wrap)
			got := MakeCompletion(reader)
			got.rawToken = "" // Compare the public parser state.
			got.canClose = false
			got.finished = false
			if len(got.Keys) == 0 {
				got.Keys = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("MakeCompletion = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCompletionFieldsTags(t *testing.T) {
	type Embedded struct {
		Promoted int `json:"promoted" help:"Embedded field"`
	}
	type Extra struct {
		Enabled bool `json:"enabled" help:"Enable the service"`
	}
	type config struct {
		Embedded
		Extra    Extra  `json:",embed"`
		Override int    `json:"promoted" help:"Direct field"`
		Name     string `json:"name,omitempty" help:"Service name"`
		Count    int    `json:",omitzero"`
		Spaced   string `json:"two words" help:"Quoted name"`
		Reserved string `json:"true" help:"Reserved name"`
		Comma    string `json:"'a,b',omitempty" help:"Comma in name"`
		Ignored  string `json:"-" help:"Hidden"`
		private  string
	}
	want := map[string]string{
		"promoted:":    "Direct field",
		"enabled:":     "Enable the service",
		"name:":        "Service name",
		"Count:":       "",
		`"two words":`: "Quoted name",
		`"true":`:      "Reserved name",
		`"a,b":`:       "Comma in name",
	}
	for _, s := range []any{config{}, (*config)(nil), reflect.TypeFor[config]()} {
		got := map[string]string{}
		Completion{Pointer: "/"}.Complete(s, got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%T: got %v, want %v", s, got, want)
		}
		got = map[string]string{"existing": "Keep this entry"}
		Completion{Pointer: "/", Token: "na"}.Complete(s, got)
		if want := map[string]string{"existing": "Keep this entry", "name:": "Service name"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("prefix completion = %v, want %v", got, want)
		}
	}
}

func TestCompletionFieldsPointers(t *testing.T) {
	type server struct {
		Host string `json:"host" help:"Server host"`
	}
	type node struct {
		Label string `json:"label" help:"Node label"`
		Next  *node  `json:"next"`
	}
	type config struct {
		Server  *server            `json:"server"`
		Servers []server           `json:"servers"`
		Fixed   [1]server          `json:"fixed"`
		ByName  map[string]*server `json:"byName"`
		Escaped server             `json:"a/b~c"`
		Node    *node              `json:"node"`
	}
	for _, tt := range []struct{ pointer, prefix, completion, description string }{
		{"/server/", "ho", "host", "Server host"},
		{"/servers/3/", "ho", "host", "Server host"},
		{"/fixed/0/", "ho", "host", "Server host"},
		{"/byName/primary/", "ho", "host", "Server host"},
		{"/a~1b~0c/", "ho", "host", "Server host"},
		{"/node/next/next/", "la", "label", "Node label"},
		{"/server/", "missing", "", ""},
		{"/missing/", "", "", ""},
		{"/server", "", "", ""},
		{"/servers/no/", "", "", ""},
		{"/fixed/1/", "", "", ""},
		{"invalid", "", "", ""},
	} {
		t.Run(tt.pointer+tt.prefix, func(t *testing.T) {
			got, want := map[string]string{}, map[string]string{}
			Completion{Pointer: tt.pointer, Token: tt.prefix}.Complete((*config)(nil), got)
			if tt.completion != "" {
				want[tt.completion+":"] = tt.description
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
	for _, s := range []any{nil, 42, []string{}} {
		got := map[string]string{}
		Completion{Pointer: "/"}.Complete(s, got)
		if len(got) != 0 {
			t.Fatalf("non-struct %T completed to %v", s, got)
		}
	}
	Completion{Pointer: "/"}.Complete((*config)(nil), nil)
}

func TestCompletionMapKeys(t *testing.T) {
	type node struct {
		Children map[string]node `json:"children"`
	}
	for _, tt := range []struct {
		input string
		want  map[string]string
	}{
		{`children:{`, map[string]string{"_:": "/children/_", "string:": "/children/string", "}": "close"}},
		{`children:{api`, map[string]string{"api:": "/children/api"}},
		{`children:{'two words`, map[string]string{`"two words":`: "/children/two words"}},
		{`children:{'a/b~c`, map[string]string{`"a/b~c":`: "/children/a~1b~0c"}},
		{`children:{_:{} `, map[string]string{"string:": "/children/string", "}": "close"}},
		{`children:{api:{} api`, map[string]string{}},
		{`children:{api:{children:{`, map[string]string{"_:": "/children/api/children/_", "string:": "/children/api/children/string", "}}}": "close"}},
	} {
		t.Run(tt.input, func(t *testing.T) {
			reader := NewRelaxedReader(strings.NewReader(tt.input))
			reader.Wrap('{')
			completion := MakeCompletion(reader)
			got := map[string]string{}
			completion.Complete(node{}, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Complete = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletionValues(t *testing.T) {
	type config struct {
		Map     map[string]map[string]int `json:"map"`
		Pointer *bool                     `json:"pointer"`
		Servers [1]map[string]string      `json:"servers"`
		Slice   []bool                    `json:"slice"`
		Escaped bool                      `json:"a/b~c"`
		Empty   int                       `json:"''"`
	}
	for _, tt := range []struct {
		name, pointer, prefix string
		root                  any
		want                  []string
	}{
		{name: "map field", root: config{}, pointer: "/map", want: []string{"{}", "null"}},
		{name: "nested map", root: (*config)(nil), pointer: "/map/primary", want: []string{"{}", "null"}},
		{name: "empty map key", root: reflect.TypeFor[config](), pointer: "/map/", want: []string{"{}", "null"}},
		{name: "pointer", root: config{}, pointer: "/pointer", want: []string{"true", "false", "null"}},
		{name: "array", root: config{}, pointer: "/servers", want: []string{"["}},
		{name: "array element", root: config{}, pointer: "/servers/0", want: []string{"{}", "null"}},
		{name: "slice", root: config{}, pointer: "/slice", want: []string{"[", "null"}},
		{name: "slice element", root: config{}, pointer: "/slice/3", want: []string{"true", "false"}},
		{name: "escaped name", root: config{}, pointer: "/a~1b~0c", want: []string{"true", "false"}},
		{name: "empty field name", root: config{}, pointer: "/", want: []string{"0", "1"}},
		{name: "empty struct", root: struct{}{}, want: []string{"{}"}},
		{name: "root map", root: map[int]string{}, want: []string{"{}", "null"}},
		{name: "boolean prefix", root: true, prefix: "t", want: []string{"true"}},
		{name: "null prefix", root: config{}, pointer: "/map", prefix: "nu", want: []string{"null"}},
		{name: "string", root: "", want: []string{"_", "string"}},
		{name: "string prefix", root: "", prefix: "hello", want: []string{"hello"}},
		{name: "string pointer", root: (*string)(nil), want: []string{"_", "string", "null"}},
		{name: "integer", root: 0, want: []string{"0", "1"}},
		{name: "unsigned", root: uint64(0), want: []string{"0", "1"}},
		{name: "float", root: float32(0), want: []string{"0", "1"}},
		{name: "bytes", root: []byte{}, want: []string{`""`, "null"}},
		{name: "byte array", root: [4]byte{}, want: []string{`"`}},
		{name: "empty byte array", root: [0]byte{}, want: []string{`""`}},
		{name: "text value", root: time.Time{}, want: []string{`"`}},
		{name: "text pointer", root: (*time.Time)(nil), want: []string{`"`, "null"}},
		{name: "any", root: reflect.TypeFor[any](), want: []string{"{}", "[", "true", "false", `""`, "0", "null"}},
		{name: "unsupported", root: make(chan int)},
		{name: "nil"},
		{name: "unknown field", root: config{}, pointer: "/missing"},
		{name: "array bounds", root: config{}, pointer: "/servers/1"},
		{name: "noncanonical index", root: config{}, pointer: "/slice/01"},
		{name: "invalid pointer", root: config{}, pointer: "invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, want := map[string]string{"existing": "Keep this entry"}, map[string]string{"existing": "Keep this entry"}
			for _, value := range tt.want {
				want[value] = tt.pointer
			}
			completion := Completion{Pointer: tt.pointer, Token: tt.prefix, Value: true}
			completion.Complete(tt.root, got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Complete = %v, want %v", got, want)
			}
			completion.Complete(tt.root, nil)
		})
	}
}

func TestCompletionValuesStruct(t *testing.T) {
	type server struct {
		Host   string `json:"host" help:"Server host"`
		True   string `json:"true"`
		Hidden string `json:"-"`
	}
	type config struct {
		Server  *server           `json:"server"`
		Servers []server          `json:"servers"`
		ByName  map[string]server `json:"by_name"`
	}
	for _, tt := range []struct {
		name, pointer, prefix string
		root                  any
		nullable              bool
	}{
		{name: "root", root: server{}},
		{name: "root pointer", root: (*server)(nil), nullable: true},
		{name: "nested pointer", root: reflect.TypeFor[config](), pointer: "/server", nullable: true},
		{name: "slice element", root: config{}, pointer: "/servers/0"},
		{name: "map value", root: config{}, pointer: "/by_name/primary"},
		{name: "field prefix", root: server{}, prefix: "{ho"},
		{name: "null prefix", root: (*server)(nil), prefix: "nu", nullable: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := map[string]string{"{}": tt.pointer, "{host:": "Server host", `{"true":`: ""}
			if tt.nullable {
				want["null"] = tt.pointer
			}
			for candidate := range want {
				if !strings.HasPrefix(candidate, tt.prefix) {
					delete(want, candidate)
				}
			}
			got := map[string]string{}
			Completion{Pointer: tt.pointer, Token: tt.prefix, Value: true}.Complete(tt.root, got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Complete = %v, want %v", got, want)
			}
		})
	}
}

func TestCompletionComplete(t *testing.T) {
	type config struct {
		Enabled bool `json:"enabled" help:"Enable service"`
	}
	for _, tt := range []struct {
		input string
		want  map[string]string
	}{
		{"", map[string]string{"{}": "", "{enabled:": "Enable service"}},
		{"{", map[string]string{"enabled:": "Enable service", "}": "close"}},
		{"{enabled:", map[string]string{"true": "/enabled", "false": "/enabled"}},
		{"{enabled:true ", map[string]string{"}": "close"}},
		{"{enabled:true}", map[string]string{"": ""}},
		{"{/* unfinished", map[string]string{}},
	} {
		t.Run(tt.input, func(t *testing.T) {
			completion := MakeCompletion(NewRelaxedReader(strings.NewReader(tt.input)))
			got := map[string]string{}
			completion.Complete(config{}, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Complete = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletionFinishAndClosers(t *testing.T) {
	type server struct {
		Host  string `json:"host"`
		Ports []int  `json:"ports"`
	}
	type config struct {
		Name    string   `json:"name"`
		Enabled bool     `json:"enabled"`
		Ports   []int    `json:"ports"`
		Server  server   `json:"server"`
		Servers []server `json:"servers"`
	}
	for _, tt := range []struct {
		name, input string
		wrap        byte
		root        any
		opts        []json.Options
		want        map[string]string
	}{
		{name: "finished object", input: "server:{host:a}", wrap: '{', want: map[string]string{"": ""}},
		{name: "finished array", input: "ports:[]", wrap: '{', want: map[string]string{"": ""}},
		{name: "finished document", input: "{}", want: map[string]string{"": ""}},
		{name: "finished nested object", input: "servers:[{}", wrap: '{', want: map[string]string{"": "", "]": "close"}},
		{name: "finished nested array", input: "server:{ports:[]", wrap: '{', want: map[string]string{"": "", "}": "close"}},
		{name: "string closer", input: "name:'hel", wrap: '{', want: map[string]string{"hel": "/name", "'hel'": "close"}},
		{name: "escaped string closer", input: "name:'\\u0061", wrap: '{', want: map[string]string{"a": "/name", "'\\u0061'": "close"}},
		{name: "nested closer", input: "server:{host:a", wrap: '{', want: map[string]string{"a": "/server/host", "a}": "close"}},
		{name: "after trivia", input: "ports:[1 /* done */", wrap: '{', want: map[string]string{"0": "/ports/1", "1": "/ports/1", "]": "close"}},
		{name: "wrong value type still closes", input: "{enabled:hello", want: map[string]string{"hello}": "close"}},
		{name: "invalid number", input: "{ports:[-", want: map[string]string{}},
		{name: "missing value", input: "{enabled:", want: map[string]string{"true": "/enabled", "false": "/enabled"}},
		{name: "unfinished key", input: "{name", want: map[string]string{"name:": ""}},
		{name: "unknown allowed", input: "{unknown:1", want: map[string]string{"1}": "close"}},
		{name: "semantic options do not restrict closers", input: "{unknown:1", opts: []json.Options{json.RejectUnknownMembers(true)}, want: map[string]string{"1}": "close"}},
		{name: "duplicates allowed by default", input: "{name:a name:b", want: map[string]string{"b": "/name", "b}": "close"}},
		{name: "duplicates allowed", input: "{name:a name:b", opts: []json.Options{jsontext.AllowDuplicateNames(true)}, want: map[string]string{"b": "/name", "b}": "close"}},
		{name: "array wrapping", input: "[1", wrap: '[', root: reflect.TypeFor[[][]int](), want: map[string]string{"1": "/0/0", "1]": "close"}},
		{name: "pointer root", input: "name:'hel", wrap: '{', root: (*config)(nil), want: map[string]string{"hel": "/name", "'hel'": "close"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewRelaxedReader(strings.NewReader(tt.input))
			reader.Wrap(tt.wrap)
			completion := MakeCompletion(reader, tt.opts...)
			root := tt.root
			if root == nil {
				root = config{}
			}
			got := map[string]string{}
			completion.Complete(root, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Complete = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompletionClosability(t *testing.T) {
	for _, tt := range []struct {
		input string
		wrap  byte
		opts  []jsontext.Options
		want  string
	}{
		{input: `{`, want: `}`},
		{input: `[`, want: `]`},
		{input: `{a:1 `, want: `}`},
		{input: `{a   `},
		{input: `{a:`},
		{input: `{a: /* done */`},
		{input: `[1 `, want: `]`},
		{input: `[   `, want: `]`},
		{input: `[1e `},
		{input: `[1e/* done */`},
		{input: `[1,`, want: `]`},
		{input: `{a:1,`, want: `}`},
		{input: `[,`},
		{input: `{,`},
		{input: `[1,,`},
		{input: `{a:1,,`},
		{input: `[1:`},
		{input: `[?`},
		{input: `{a:[{`, want: `}]}`},
		{input: `{a:[{b:1 `, want: `}]}`},
		{input: `{a:[{b   `},
		{input: `{a:[{b:`},
		{input: `{a:[{} `, want: `]}`},
		{input: `{a:[{}]`, want: `}`},
		{input: `'`, want: `''`},
		{input: `"root`, want: `"root"`},
		{input: `{a:'`, want: `''}`},
		{input: `{a:['x`, want: `'x']}`},
		{input: `{a:['\u0061`, want: `'\u0061']}`},
		{input: `{a:"\u0061"`, want: `"\u0061"}`},
		{input: `{a:'it\'s'`, want: `'it\'s'}`},
		{input: `{a:'a"b`, want: `'a"b'}`},
		{input: `{a:1.00e+02`, want: `1.00e+02}`},
		{input: `{a:['\u12`},
		{input: `{a:['x\`},
		{input: `{a:['x\'`, want: `'x\'']}`},
		{input: `{'name`},
		{input: `[1 /* unfinished`},
		{input: `[1 // comment`},
		{input: "[1 // comment\n", want: `]`},
		{input: `a:1`, wrap: '{'},
		{input: `[   `, wrap: '{'},
		{input: `a:[{b:1 `, wrap: '{', want: `}]`},
		{input: `{a:1 a:2 `, want: `}`},
		{input: `{a:1 a:2 `, opts: []jsontext.Options{jsontext.AllowDuplicateNames(false)}},
	} {
		t.Run(tt.input, func(t *testing.T) {
			reader := NewRelaxedReader(strings.NewReader(tt.input))
			reader.Wrap(tt.wrap)
			completion := MakeCompletion(reader, tt.opts...)
			completions := map[string]string{}
			completion.Complete(reflect.TypeFor[any](), completions)
			var got []string
			for replacement, note := range completions {
				if note == "close" {
					got = append(got, replacement)
				}
			}
			var want []string
			if tt.want != "" {
				want = []string{tt.want}
			}
			if !slices.Equal(got, want) {
				t.Fatalf("closers = %q, want %q", got, want)
			}
		})
	}
}

func FuzzMakeCompletion(f *testing.F) {
	for _, seed := range []string{
		``, `{a:[0 `, `{'a/b~c': ['partial`, `{"a":"\u12`, `[1 2e-`, `{a:1 a`, `[1 ?`,
		`{name:"x"en`, `{name:'x'/* done */en`, `{a:1 # comment`, `{a:1, /* unfinished`,
		`{a:1 `, `{a   `, `[1e `, `[   `, `{a:[{b:1 `, `{a:['x\'`, `{a:1 a:2 `,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		original := bytes.Clone(input)
		for _, wrap := range []byte{0, '{', '['} {
			reader := NewRelaxedReader(bytes.NewReader(input))
			reader.Wrap(wrap)
			got := MakeCompletion(reader)
			if !jsontext.Pointer(got.Pointer).IsValid() {
				t.Fatalf("wrap %q: invalid completion pointer %q for %q", wrap, got.Pointer, input)
			}
			if got.Start < -1 || got.Start > len(input) {
				t.Fatalf("wrap %q: invalid token offset %d for %q", wrap, got.Start, input)
			}
			completions := map[string]string{}
			got.Complete(reflect.TypeFor[any](), completions)
			for replacement, note := range completions {
				if note != "close" {
					continue
				}
				if want := string(input[got.Start:]) + got.Closer; replacement != want {
					t.Fatalf("wrap %q: closer %q changes source spelling, want %q", wrap, replacement, want)
				}
				completed := append(bytes.Clone(input[:got.Start]), replacement...)
				reader := NewRelaxedReader(bytes.NewReader(completed))
				reader.Wrap(got.Wrap)
				if err := json.UnmarshalRead(reader, new(jsontext.Value), jsontext.AllowDuplicateNames(true)); err != nil {
					t.Fatalf("wrap %q: closer %q for %q is invalid: %v", wrap, replacement, input, err)
				}
			}
		}
		if !bytes.Equal(input, original) {
			t.Fatal("MakeCompletion modified the input")
		}
	})
}
