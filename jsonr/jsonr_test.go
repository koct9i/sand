package jsonr

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func TestUnmarshalRelaxed(t *testing.T) {
	tests := []struct {
		name, input, strict string
	}{
		{"standard", `{"a":[true,false,null,-1.25e+3],"b":{}}`, `{"a":[true,false,null,-1250],"b":{}}`},
		{"comments", "# config\n{/* block */ a: 1, // line\n b: 2 # hash\r}", `{"a":1,"b":2}`},
		{"trailing commas", `{a:[1,2,/*done*/],b:{c:3,},}`, `{"a":[1,2],"b":{"c":3}}`},
		{"optional array commas", `[1 2,true false null bare 'x'"y" [] {} [3 4]]`, `[1,2,true,false,null,"bare","x","y",[],{},[3,4]]`},
		{"optional object commas", `{a:1 b:[true false] c:{x:bare y:'text'} d:[] e:{} f:null}`, `{"a":1,"b":[true,false],"c":{"x":"bare","y":"text"},"d":[],"e":{},"f":null}`},
		{"adjacent containers", `[{}[]{a:1}{b:2}[1][2]]`, `[{},[],{"a":1},{"b":2},[1],[2]]`},
		{"escaped punctuation", `\{a\:\[1\,2\,\] b\:\{c\:true\}\}`, `{"a":[1,2],"b":{"c":true}}`},
		{"escapes inside strings", `["\\:" '\\{' "\\}" '\\[' "\\]"]`, `["\\:","\\{","\\}","\\[","\\]"]`},
		{"comments replacing commas", "[1/*comment*/2 //line\n3 #hash\n4]", `[1,2,3,4]`},
		{"comments split bare tokens", `[tr/*comment*/ue 1/*comment*/e2]`, `["tr","ue",1,"e2"]`},
		{"mixed separators", `{a:[1,2 3,] b:4,c:5}`, `{"a":[1,2,3],"b":4,"c":5}`},
		{"quotes", `{'a':'it\'s "fine"', b:'\\\n\t\b\f\r\/\u0041',}`, `{"a":"it's \"fine\"","b":"\\\n\t\b\f\r/A"}`},
		{"identifiers", `{_a:1,$b2:2,'true':3,'null':4,'false':5}`, `{"_a":1,"$b2":2,"true":3,"null":4,"false":5}`},
		{"bare values", `{mode:fast,nested:[on,off,true,false,null,-12,3.5e2],}`, `{"mode":"fast","nested":["on","off",true,false,null,-12,350]}`},
		{"root bare string", `hello`, `"hello"`},
		{"bare identifiers", `[bare,undefined,NaN,Infinity,truefalse]`, `["bare","undefined","NaN","Infinity","truefalse"]`},
		{"quoted names", `{'':0,'123':1,'a-b':2,'日本語':3}`, `{"":0,"123":1,"a-b":2,"日本語":3}`},
		{"comment text in strings", `{url:'https://example/#hash', text:"/*literal*/ // #", q:'\"'}`, `{"url":"https://example/#hash","text":"/*literal*/ // #","q":"\""}`},
		{"between tokens", `{a/*one*/:/*two*/[/*three*/1/*four*/,/*five*/2/*six*/]/*seven*/}`, `{"a":[1,2]}`},
		{"nested empties", `{a:[{},[],{b:[],},],}`, `{"a":[{},[],{"b":[]}]}`},
		{"root string", `'hello' # done`, `"hello"`},
		{"root number", `123/*done*/`, `123`},
		{"root bool", `true//done`, `true`},
		{"root null", `null#done`, `null`},
		{"empty containers", `/* start */ {a:[/* nothing */],b:{# nothing
}} // end`, `{"a":[],"b":{}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var want any
			if err := json.Unmarshal([]byte(tt.strict), &want); err != nil {
				t.Fatal(err)
			}
			var got any
			if err := Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestUnmarshalRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{
		``, `# comment only`, `/* unfinished`, `/*/`, `/`,
		`{a:1`, `'unfinished`, `'unfinished\`, `[1,/*unfinished]`,
		`[,]`, `{,}`, `[1,,]`, `{a:1,,}`, `{a:,}`, `{a 1}`, `{a::1}`,
		`{a-b:1}`, `{123:1}`, `{é:1}`, `{a\u0062:1}`,
		`{true:1}`, `{false:2}`, `{null:3}`,
		`+1`, `.5`, `01`, `0x10`, `1e`,
		`'\x41'`, `'\q'`, `"\'"`, "'line\nbreak'", "'line\\\nbreak'",
		`\`, `\q`, `\\{}`, `"\:"`, `'\{'`, `\{a\:1`,
		`{} {}`, `1 2`, `true false`, `[] garbage`, `{} /* unfinished`, `{} ,`,
	} {
		t.Run(input, func(t *testing.T) {
			var got any
			if err := Unmarshal([]byte(input), &got); err == nil {
				t.Fatalf("accepted malformed input as %#v", got)
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	limit := strings.Repeat("a", 256)
	long := limit + "a"
	for _, tt := range []struct {
		name, input, want string
		indent            string
	}{
		{"bare names", `{"a":1,"_b":2,"$c3":{"nested":[{"d":4}]}}`, `{a:1 _b:2 $c3:{nested:[{d:4}]}}`, ""},
		{"quoted names", `{"":0,"123":1,"a-b":2,"日本語":3,"a\"b":4,"a\\b":5}`, `{"":0 "123":1 "a-b":2 "日本語":3 "a\"b":4 "a\\b":5}`, ""},
		{"reserved names", `{"true":true,"false":false,"null":null,"truefalse":"true"}`, `{"true":true "false":false "null":null truefalse:"true"}`, ""},
		{"escaped names", `{"\u0061":1,"\u0024b":2,"tr\u0075e":3}`, `{a:1 $b:2 "true":3}`, ""},
		{"string values", `{"bare":"bare","quote":"a\"b","slash":"a\\b","syntax":": , // # /* */"}`, `{bare:bare quote:"a\"b" slash:"a\\b" syntax:": , // # /* */"}`, ""},
		{"strings at limit", `{"` + limit + `":"` + limit + `"}`, `{` + limit + `:` + limit + `}`, ""},
		{"long strings", `{"` + long + `":"` + long + `"}`, `{"` + long + `":"` + long + `"}`, ""},
		{"root string", `"a,b"`, `"a,b"`, ""},
		{"numbers", `{"n":1e+100,"zero":-0}`, `{n:1e+100 zero:-0}`, ""},
		{"array values", `[true,false,null,1,2,-3,1.5,1e2,"a,b",{},[]]`, `[true false null 1 2 -3 1.5 1e2 "a,b" {} []]`, ""},
		{"nested containers", `[{"a":[]},{"b":[1,2]},[{},[]]]`, `[{a:[]} {b:[1 2]} [{} []]]`, ""},
		{"indentation", `{"a":[1,2],"b":true}`, "{\n  a: [\n    1\n    2\n  ]\n  b: true\n}", "  "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []json.Options
			if tt.indent != "" {
				opts = append(opts, jsontext.WithIndent(tt.indent))
			}
			got, err := Marshal(jsontext.Value(tt.input), opts...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("Marshal = %s, want %s", got, tt.want)
			}
			var decoded, want any
			if err := json.Unmarshal([]byte(tt.input), &want); err != nil {
				t.Fatal(err)
			}
			if err := Unmarshal(got, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, want) {
				t.Fatalf("round trip = %#v, want %#v", decoded, want)
			}
		})
	}
}

func TestRelaxedWriter(t *testing.T) {
	limit := strings.Repeat("a", 256)
	long := limit + "a"
	escapedLong := strings.Repeat("a", 251) + `\u0062`
	escapedQuote := strings.Repeat("a", 255) + `\",b`
	for _, tt := range []struct{ name, input, want string }{
		{"nested", `{"a":[true,false,null,1,-2,{}],"b":{"c":[]},"d":3}`, `{a:[true false null 1 -2 {}] b:{c:[]} d:3}`},
		{"bare values", `["bare","$x_2","true","false","null","123","","\u0061"]`, `[bare $x_2 "true" "false" "null" "123" "" a]`},
		{"strings", `["a,b","a\"b\\c",{"true":"null","a-b":"x,y","a\"b":1}]`, `["a,b" "a\"b\\c" {"true":"null" "a-b":"x,y" "a\"b":1}]`},
		{"strings at limit", `{"` + limit + `":"` + limit + `"}`, `{` + limit + `:` + limit + `}`},
		{"long strings", `{"` + long + `":"` + long + `","next":"short"}`, `{"` + long + `":"` + long + `" next:short}`},
		{"escape at limit", `"` + strings.Repeat("a", 250) + `\u0062"`, strings.Repeat("a", 250) + "b"},
		{"escape over limit", `"` + escapedLong + `"`, `"` + escapedLong + `"`},
		{"escape crossing limit", `"` + escapedQuote + `"`, `"` + escapedQuote + `"`},
		{"escaped names", `{"\u0061":1,"\u0024b":2,"tr\u0075e":3}`, `{a:1 $b:2 "tr\u0075e":3}`},
		{"whitespace", "{\n  \"a\" : [1, 2],\n  \"b\": {\"c\": 3}\n}", "{\n  a : [1 2]\n  b: {c: 3}\n}"},
		{"root string", `"a\"b,c"`, `"a\"b,c"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, size := range []int{1, 7, len(tt.input)} {
				var out bytes.Buffer
				w := NewRelaxedWriter(&out)
				input := []byte(tt.input)
				for i := 0; i < len(input); i += size {
					p := input[i:min(i+size, len(input))]
					if n, err := w.Write(p); n != len(p) || err != nil {
						t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(p))
					}
				}
				if out.String() != tt.want {
					t.Fatalf("chunk size %d: got %q, want %q", size, &out, tt.want)
				}
				if string(input) != tt.input {
					t.Fatal("Write modified the input")
				}
			}
		})
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestRelaxedWriterErrors(t *testing.T) {
	for _, outputErr := range []error{errors.New("output failed"), nil} {
		wantErr := outputErr
		if wantErr == nil {
			wantErr = io.ErrShortWrite
		}
		calls := 0
		w := NewRelaxedWriter(writerFunc(func(p []byte) (int, error) {
			calls++
			return 0, outputErr
		}))
		if _, err := w.Write([]byte(`{"a":1}`)); !errors.Is(err, wantErr) {
			t.Fatalf("Write error = %v, want %v", err, wantErr)
		}
		if n, err := w.Write([]byte(`[]`)); n != 0 || !errors.Is(err, wantErr) || calls != 1 {
			t.Fatalf("Write after error = %d, %v; output calls = %d", n, err, calls)
		}
	}
}

func TestRelaxedReaderWrap(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		wrap              byte
	}{
		{"empty array", ``, `[]`, '['},
		{"empty object", ``, `{}`, '{'},
		{"object whitespace", " \t\r\n", `{}`, '{'},
		{"whitespace", " \t\r\n", `[]`, '['},
		{"comment only", `# empty`, `{}`, '{'},
		{"comments only", "/* empty */ // empty\n", `[]`, '['},
		{"object comments only", "/* empty */ // empty\n", `{}`, '{'},
		{"explicit empty object", `{}`, `{}`, '{'},
		{"escaped explicit object", `\{a\:1\}`, `{"a":1}`, '{'},
		{"escaped implicit object", `a\:1 b\:\{c\:true\}`, `{"a":1,"b":{"c":true}}`, '{'},
		{"explicit object", `{a:1 b:[2 3]}`, `{"a":1,"b":[2,3]}`, '{'},
		{"explicit object after whitespace", " \t\r\n{a:1}", `{"a":1}`, '{'},
		{"explicit object after comments", "# config\n/* leading */ // note\n{a:1,} # done", `{"a":1}`, '{'},
		{"brace inside leading comment", "/* { */ # {\na:1", `{"a":1}`, '{'},
		{"array wrapping stays unconditional", `[1 2]`, `[[1,2]]`, '['},
		{"escaped array wrapping", `\[1 2\]`, `[[1,2]]`, '['},
		{"number at EOF", `42`, `[42]`, '['},
		{"bare string at EOF", `bare`, `["bare"]`, '['},
		{"literal at EOF", `true`, `[true]`, '['},
		{"quoted string at EOF", `'text'`, `["text"]`, '['},
		{"array values", `1 true null bare 'text' [2 3] {a:4}`, `[1,true,null,"bare","text",[2,3],{"a":4}]`, '['},
		{"adjacent containers", `{}[]{a:1}[2]`, `[{},[],{"a":1},[2]]`, '['},
		{"mixed separators", `1,2 3,`, `[1,2,3]`, '['},
		{"trailing whitespace", "1 2 \n", `[1,2]`, '['},
		{"trailing block comment", `1 2 /* done */`, `[1,2]`, '['},
		{"trailing line comment", `1 2 // done`, `[1,2]`, '['},
		{"trailing comma and comment", `1,2, # done`, `[1,2]`, '['},
		{"object number at EOF", `a:1`, `{"a":1}`, '{'},
		{"object bare string at EOF", `a:bare`, `{"a":"bare"}`, '{'},
		{"object members", `a:1 b:[2 3] 'c-d':{e:'text',}`, `{"a":1,"b":[2,3],"c-d":{"e":"text"}}`, '{'},
		{"object trailing comma", `a:1,b:2,`, `{"a":1,"b":2}`, '{'},
		{"object comments", "# config\na/* key */:bare // next\nb:true, /* done */", `{"a":"bare","b":true}`, '{'},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRelaxedReader(strings.NewReader(tt.input))
			r.Wrap(tt.wrap)
			var got, want any
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.UnmarshalRead(r, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, want %#v", got, want)
			}
			if n, err := r.Read(make([]byte, 1)); n != 0 || err != io.EOF {
				t.Fatalf("Read after EOF = %d, %v", n, err)
			}
			// Exercise both input and output boundaries while detecting the first token.
			r = NewRelaxedReader(iotest.OneByteReader(strings.NewReader(tt.input)))
			r.Wrap(tt.wrap)
			output, err := io.ReadAll(iotest.OneByteReader(r))
			if err != nil {
				t.Fatal(err)
			}
			got = nil
			if err := json.Unmarshal(output, &got); err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("one byte reads: output %q, value %#v, error %v; want %#v", output, got, err, want)
			}
		})
	}
}

func TestRelaxedReaderTrivia(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		wrap              byte
	}{
		{"unwrapped", " \t\r\n/* note */ # note\n{a:1} \n", `{"a":1}`, 0},
		{"between numbers", `[1/*xx*/2]`, `[1,2]`, 0},
		{"escaped punctuation", `\{a\:\[1\,2\] b\:\{\}\}`, `{"a":[1,2],"b":{}}`, 0},
		{"escaped adjacent containers", `\[1\{a\:2\}\[3\]\]`, `[1,{"a":2},[3]]`, 0},
		{"escaped implicit object", `a\:true`, `{"a":true}`, '{'},
		{"escaped trailing comma", `\{a\:1\,/* done */\}`, `{"a":1 }`, 0},
		{"escaped trailing comma at EOF", `a\:1\,`, `{"a":1 }`, '{'},
		{"escaped punctuation in strings", `["\\:" '\\{' "\\}" '\\[' "\\]"]`, `["\\:","\\{","\\}","\\[","\\]"]`, 0},
		{"escaped punctuation in comments", `/* \{ \: */\{a\:1\}`, `{"a":1}`, 0},
		{"between bare tokens", `[tr/*xx*/ue]`, `["tr","ue"]`, 0},
		{"object separators", `{ /* note */ a /* note */ : [ 1 /* note */ 2 ] b : ' x y ' }`, `{"a":[1,2],"b":" x y "}`, 0},
		{"implicit object", " // note\n a:1", `{"a":1}`, '{'},
		{"explicit object", " \t/* note */{a:1}", `{"a":1}`, '{'},
		{"quoted brace", " \t'{':1", `{"{":1}`, '{'},
		{"number", " \t/* note */1", `[1]`, '['},
		{"literal", " \ttrue", `[true]`, '['},
		{"string content", " \t/* note */' x'", `[" x"]`, '['},
		{"comments only", " // note\n/* note */", ``, 0},
		{"empty object", " // note\n/* note */", `{}`, '{'},
		{"empty array", " \t\r\n", `[]`, '['},
		{"long prefix", strings.Repeat(" \t\n", 4096) + "/* note */{a:1}", `{"a":1}`, '{'},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRelaxedReader(iotest.OneByteReader(strings.NewReader(tt.input)))
			r.Wrap(tt.wrap)
			output, err := io.ReadAll(r)
			if err != nil || string(output) != tt.want {
				t.Fatalf("ReadAll = %q, %v; want %q, nil", output, err, tt.want)
			}
		})
	}
}

func TestRelaxedReaderWrapRejectsMalformedInput(t *testing.T) {
	for _, tt := range []struct {
		wrap  byte
		input string
	}{
		{'[', `,`}, {'[', `,1`}, {'[', `1,,`}, {'[', `'unfinished`},
		{'[', `1 /* unfinished`}, {'[', `[1`}, {'[', `]`},
		{'{', `,`}, {'{', `a`}, {'{', `a:`}, {'{', `a:,`},
		{'{', `a 1`}, {'{', `a:1,,`}, {'{', `a:{b:1`}, {'{', `true:1`},
		{'{', `{a:1`}, {'{', `/* leading */ {a:1`}, {'{', `{} {}`},
		{'{', `{},`}, {'{', `{a:1} garbage`}, {'{', `/* unfinished`},
	} {
		t.Run(string(tt.wrap)+tt.input, func(t *testing.T) {
			r := NewRelaxedReader(strings.NewReader(tt.input))
			r.Wrap(tt.wrap)
			if err := json.UnmarshalRead(r, new(any)); err == nil {
				t.Fatal("accepted malformed input")
			}
		})
	}
}

func TestSourceErrorOffsets(t *testing.T) {
	for _, prefix := range []string{
		`/* leading 日本 */`, "# leading\n",
		" \t\n", " \t/* leading 日本 */",
		`{a:'a"b\'c',/* comment */bad:`,
		`/* comment */{a:[1 2] b:{c:3 d:4} bad:`,
		`\{a\:1 b\:\{c\:true\} bad\:`,
	} {
		t.Run(prefix, func(t *testing.T) {
			for _, wrap := range []byte{0, '{', '['} {
				r := NewRelaxedReader(strings.NewReader(prefix + "?}"))
				r.Wrap(wrap)
				err := r.SourceError(json.UnmarshalRead(r, new(any)))
				var syntax *jsontext.SyntacticError
				if !errors.As(err, &syntax) || syntax.ByteOffset != int64(len(prefix)) {
					t.Fatalf("wrap %q: got %v, want syntax error at source byte %d", wrap, err, len(prefix))
				}
			}
		})
	}
	for _, input := range []string{
		`[1 `, `[1 /* done */`, "{a: \n/* done */",
		`\{a\:\{b\:1`,
	} {
		t.Run(input, func(t *testing.T) {
			err := Unmarshal([]byte(input), new(any))
			var syntax *jsontext.SyntacticError
			if !errors.As(err, &syntax) || syntax.ByteOffset != int64(len(input)) {
				t.Fatalf("got %v, want syntax error at source byte %d", err, len(input))
			}
		})
	}
}

func FuzzStrictCompatibility(f *testing.F) {
	for _, seed := range []string{
		`null`, `1e+100`, `-0`, `"\\\""`, `{"a":[1,"text",{},[]]}`,
		`{"\u0061":"\uD834\uDD1E"}`, `{"0":true,"":false}`, `{"a":1,"a":2}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var want, got any
		if err := json.Unmarshal([]byte(input), &want); err != nil {
			return
		}
		if err := Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("rejected valid JSON %q: %v", input, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("input %q: got %#v, want %#v", input, got, want)
		}
	})
}

func FuzzRelaxedRoundTrip(f *testing.F) {
	for _, seed := range []string{
		`{a:'hello', b:[1,2,],}`, `# comment
{a:'it\'s fine',}`, `"text"`, `123`, `[{nested:{a:1,},},]`,
		`{a:[1 2] b:[{}[]{x:bare y:true}]}`, "[1/*comment*/2 #hash\n3]",
		`{"` + strings.Repeat("a", 256) + `":"` + strings.Repeat("b", 257) + `"}`,
		`\{a\:\[1\,2\]\}`, `\[1\{a\:2\}\[3\]\]`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var want any
		if err := Unmarshal([]byte(input), &want); err != nil {
			return
		}
		b, err := Marshal(want, json.Deterministic(true))
		if err != nil {
			t.Fatal(err)
		}
		strict, err := json.Marshal(want, json.Deterministic(true))
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		w := NewRelaxedWriter(&out)
		for i := range strict {
			if _, err := w.Write(strict[i : i+1]); err != nil {
				t.Fatal(err)
			}
		}
		if !bytes.Equal(out.Bytes(), b) {
			t.Fatalf("chunked writer = %q, want %q", &out, b)
		}
		var got any
		if err := Unmarshal(b, &got); err != nil {
			t.Fatalf("cannot decode encoded value %q: %v", b, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("input %q: round trip got %#v, want %#v", input, got, want)
		}
	})
}

func FuzzOptionalCommas(f *testing.F) {
	for _, seed := range []string{`[1,2,3]`, `{"a":[1,{},2],"b":true}`, `["a,b","a\\\"b",null,false,-1e2]`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var want, got any
		if err := json.Unmarshal([]byte(input), &want); err != nil {
			return
		}
		relaxed := []byte(input)
		for i := 0; i < len(relaxed); i++ {
			switch relaxed[i] {
			case '"':
				i++
				for i < len(relaxed) && relaxed[i] != '"' {
					if relaxed[i] == '\\' {
						i++
					}
					i++
				}
			case ',':
				relaxed[i] = ' '
			}
		}
		if err := Unmarshal(relaxed, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("omitting commas from %q: decoded %#v, %v; want %#v", input, got, err, want)
		}
	})
}
