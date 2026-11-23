package jsonr

import (
	"bytes"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGetLastToken(t *testing.T) {
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
			token, pointer, closer, _, _ := GetLastToken(strings.NewReader(tt.input))
			if token != tt.token || pointer != tt.pointer || closer != tt.closer {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", token, pointer, closer, tt.token, tt.pointer, tt.closer)
			}
		})
	}
}

func TestGetLastTokenValuePosition(t *testing.T) {
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
			_, _, _, _, value := GetLastToken(strings.NewReader(tt.input))
			if value != tt.value {
				t.Fatalf("value position = %t, want %t", value, tt.value)
			}
		})
	}
}

func TestGetLastTokenStart(t *testing.T) {
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
			_, _, _, start, _ := GetLastToken(strings.NewReader(tt.input))
			want := len(tt.input) - len(tt.source)
			if start != want {
				t.Fatalf("token offset = %d, want %d for source spelling %q", start, want, tt.source)
			}
		})
	}
	for _, input := range []string{
		`# comment`, `// comment`, `/* unfinished`, `/*/`,
		`{a:1 # comment`, `{a:1 // comment`, `{a:1 /* unfinished`,
		`{a:1, # comment`, `{a:1, // comment`, `{a:1, /* unfinished`,
	} {
		t.Run(input, func(t *testing.T) {
			_, _, _, start, _ := GetLastToken(strings.NewReader(input))
			if start != -1 {
				t.Fatalf("token offset inside a comment = %d, want -1", start)
			}
		})
	}
}

func TestStructCompletionTags(t *testing.T) {
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
		"promoted":    "Direct field",
		"enabled":     "Enable the service",
		"name":        "Service name",
		"Count":       "",
		`"two words"`: "Quoted name",
		`"true"`:      "Reserved name",
		`"a,b"`:       "Comma in name",
	}
	for _, s := range []any{config{}, (*config)(nil), reflect.TypeFor[config]()} {
		handler := StructCompletion(s)
		for _, pointer := range []string{"", "/"} {
			got := map[string]string{}
			handler(pointer, "", got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%T at %q: got %v, want %v", s, pointer, got, want)
			}
		}
		got := map[string]string{"existing": "Keep this entry"}
		handler("/", "na", got)
		if want := map[string]string{"existing": "Keep this entry", "name": "Service name"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("prefix completion = %v, want %v", got, want)
		}
	}
}

func TestStructCompletionPointers(t *testing.T) {
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
	handler := StructCompletion((*config)(nil))
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
			handler(tt.pointer, tt.prefix, got)
			if tt.completion != "" {
				want[tt.completion] = tt.description
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
	for _, s := range []any{nil, 42, []string{}} {
		got := map[string]string{}
		StructCompletion(s)("", "", got)
		if len(got) != 0 {
			t.Fatalf("non-struct %T completed to %v", s, got)
		}
	}
	handler("/", "", nil)
}

func TestValueCompletion(t *testing.T) {
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
		{name: "map field", root: config{}, pointer: "/map", want: []string{"{", "null"}},
		{name: "nested map", root: (*config)(nil), pointer: "/map/primary", want: []string{"{", "null"}},
		{name: "empty map key", root: reflect.TypeFor[config](), pointer: "/map/", want: []string{"{", "null"}},
		{name: "pointer", root: config{}, pointer: "/pointer", want: []string{"true", "false", "null"}},
		{name: "array", root: config{}, pointer: "/servers", want: []string{"["}},
		{name: "array element", root: config{}, pointer: "/servers/0", want: []string{"{", "null"}},
		{name: "slice", root: config{}, pointer: "/slice", want: []string{"[", "null"}},
		{name: "slice element", root: config{}, pointer: "/slice/3", want: []string{"true", "false"}},
		{name: "escaped name", root: config{}, pointer: "/a~1b~0c", want: []string{"true", "false"}},
		{name: "empty field name", root: config{}, pointer: "/", want: []string{"0", "1"}},
		{name: "root struct", root: config{}, want: []string{"{"}},
		{name: "root map", root: map[int]string{}, want: []string{"{", "null"}},
		{name: "boolean prefix", root: true, prefix: "t", want: []string{"true"}},
		{name: "null prefix", root: config{}, pointer: "/map", prefix: "nu", want: []string{"null"}},
		{name: "string", root: "", want: []string{"_", "string"}},
		{name: "string prefix", root: "", prefix: "hello", want: []string{"hello", "hello,"}},
		{name: "string pointer", root: (*string)(nil), want: []string{"_", "string", "null"}},
		{name: "integer", root: 0, want: []string{"0", "1"}},
		{name: "unsigned", root: uint64(0), want: []string{"0", "1"}},
		{name: "float", root: float32(0), want: []string{"0", "1"}},
		{name: "bytes", root: []byte{}, want: []string{`""`, "null"}},
		{name: "byte array", root: [4]byte{}, want: []string{`"`}},
		{name: "empty byte array", root: [0]byte{}, want: []string{`""`}},
		{name: "text value", root: time.Time{}, want: []string{`"`}},
		{name: "text pointer", root: (*time.Time)(nil), want: []string{`"`, "null"}},
		{name: "any", root: reflect.TypeFor[any](), want: []string{"{", "[", "true", "false", `""`, "0", "null"}},
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
			handler := ValueCompletion(tt.root)
			handler(tt.pointer, tt.prefix, got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ValueCompletion = %v, want %v", got, want)
			}
			handler(tt.pointer, tt.prefix, nil)
		})
	}
}

func FuzzGetLastToken(f *testing.F) {
	for _, seed := range []string{
		``, `{a:[0 `, `{'a/b~c': ['partial`, `{"a":"\u12`, `[1 2e-`, `{a:1 a`, `[1 ?`,
		`{name:"x"en`, `{name:'x'/* done */en`, `{a:1 # comment`, `{a:1, /* unfinished`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		original := bytes.Clone(input)
		_, pointer, _, start, _ := GetLastToken(bytes.NewReader(input))
		if !jsontext.Pointer(pointer).IsValid() {
			t.Fatalf("invalid completion pointer %q for %q", pointer, input)
		}
		if start < -1 || start > len(input) {
			t.Fatalf("invalid token offset %d for %q", start, input)
		}
		if !bytes.Equal(input, original) {
			t.Fatal("GetLastToken modified the input")
		}
	})
}
