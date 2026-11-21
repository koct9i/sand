package jsonr

import (
	"bytes"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"
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
		{`[1 -`, "-", "/1", "]"},
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
			token, pointer, closer := GetLastToken([]byte(tt.input))
			if token != tt.token || pointer != tt.pointer || closer != tt.closer {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", token, pointer, closer, tt.token, tt.pointer, tt.closer)
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

func TestCompletorStructCompletion(t *testing.T) {
	type server struct {
		Host string `json:"host" help:"Server host"`
	}
	type config struct {
		Name   string `json:"name" help:"Service name"`
		Server server `json:"server"`
	}
	c := Completor{Handlers: map[string]CompletionFunc{"": StructCompletion(config{})}}
	for _, tt := range []struct {
		input string
		want  map[string]string
	}{
		{`{na`, map[string]string{"name": "Service name", "na}": ""}},
		{`{server:{ho`, map[string]string{"host": "Server host", "ho}}": ""}},
		{`name`, map[string]string{"name": "Service name"}},
	} {
		if got := c.GetCompletions([]byte(tt.input)); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%q: got %v, want %v", tt.input, got, tt.want)
		}
	}
	c.Handlers["/server"] = func(pointer, prefix string, completions map[string]string) {
		if pointer != "/" || prefix != "ho" {
			t.Fatalf("handler got pointer %q, prefix %q", pointer, prefix)
		}
		completions["host"] = "Override"
	}
	if got := c.GetCompletions([]byte(`{server:{ho`)); got["host"] != "Override" {
		t.Fatalf("specific handler did not override root: %v", got)
	}
}

func FuzzGetLastToken(f *testing.F) {
	for _, seed := range []string{``, `{a:[0 `, `{'a/b~c': ['partial`, `{"a":"\u12`, `[1 2e-`, `{a:1 a`, `[1 ?`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		original := bytes.Clone(input)
		_, pointer, _ := GetLastToken(input)
		if !jsontext.Pointer(pointer).IsValid() {
			t.Fatalf("invalid completion pointer %q for %q", pointer, input)
		}
		if !bytes.Equal(input, original) {
			t.Fatal("GetLastToken modified the input")
		}
	})
}
