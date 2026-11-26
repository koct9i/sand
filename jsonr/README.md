# jsonr

A relaxed JSON encoder and decoder built on Go 1.27's `encoding/json/v2` and
`encoding/json/jsontext`. No external dependencies or experiment flags are needed.

Decoding accepts standard JSON plus:

- `//`, `/* ... */`, and `#` comments between tokens.
- Optional commas between object members and array elements, and trailing commas.
- Single-quoted strings, with JSON escapes and the additional `\'` escape.
- Backslash-escaped structural punctuation (`\{`, `\}`, `\[`, `\]`, `\:`,
  and `\,`) outside strings, for shell completion. Quoted strings keep their
  usual JSON escape rules.
- Bare strings matching `[A-Za-z_$][A-Za-z0-9_$]*`, as keys or values.
  `true`, `false`, and `null` retain their JSON meanings and require quotes
  when used as object keys or string values.

Encoding omits commas between members and elements, and omits quotes from bare
strings in both keys and values. Strings with more than 256 bytes of encoded
content remain quoted, as do reserved words and strings that are not identifiers.
Compact output uses spaces as separators; indented output keeps its whitespace.
Comments and original formatting are discarded. This is a defined subset of
relaxed syntax, not a full JSON5 implementation: numbers follow standard JSON
rules, and colons remain required. For example, `{a:1 b:[2 3]}` decodes like
`{"a":1,"b":[2,3]}`.

```go
package main

import (
    "fmt"
    "encoding/json/jsontext"
    "encoding/json/v2"

    "github.com/koct9i/junk/jsonr"
)

func main() {
    var config struct {
        Name  string `json:"name"`
        Ports []int  `json:"ports"`
    }
    err := jsonr.Unmarshal([]byte(`{
        // Local service
        name: 'example',
        ports: [8080, 8081,],
    }`), &config)
    if err != nil {
        panic(err)
    }
    b, err := jsonr.Marshal(config,
        json.Deterministic(true), jsontext.WithIndent("  "))
    if err != nil {
        panic(err)
    }
    fmt.Println(string(b))
}
```

`Marshal` produces one value without a final newline. `Unmarshal` requires exactly
one value and allows surrounding comments and whitespace.

`NewRelaxedWriter(writer)` exposes the encoding conversion as an `io.Writer`.
Pass it to `json.MarshalWrite`, or write standard JSON directly. It omits commas
and unquotes short bare strings, including when tokens span multiple writes.
It buffers at most 256 bytes of string content before keeping a string quoted.
Complete JSON values need no flush. Output errors are retained for subsequent
writes. The writer expects valid JSON and does not validate syntax.

`NewRelaxedReader(reader)` exposes the conversion as an `io.Reader`. Before
reading, call `Wrap('[')` to enclose a list of values in an array, or `Wrap('{')`
to enclose object members in an object. For example, `1 2 3,` becomes `[1,2,3]`,
and `name:'example' ports:[8080 8081]` becomes an object. Object wrapping is skipped
when the first token after whitespace and comments is already `{`; array
wrapping always adds an outer array. Empty input produces an empty container.
Whitespace and comments between tokens are discarded. Decode the result with
`json.UnmarshalRead`.

`MakeCompletion(reader)` inspects incomplete input from a `RelaxedReader` for
completion. It returns a `Completion` with the token at the cursor, its completion
position as a JSON Pointer, the closing quotes and containers still open, the
token's byte offset in the original input, including any opening quote, whether
the cursor is completing a value, and the keys already read in the current object.
Keys are unquoted, exclude the active name being completed, and are nil outside
an object. For example, input `{a:1 b` produces `Keys: []string{"a"}`.
Trailing whitespace clears the token, advances the array position to `/a/1`, and
sets the offset to the input length. An offset of `-1` indicates that input ends
inside a line or block comment, where completion should be suppressed.
After an object name and trailing whitespace or a completed comment, completion
suggests the missing `:` instead of another name.

Pass a configured `*RelaxedReader` to `MakeCompletion` to complete an implicit
object or array. `Completion.Wrap` reports the reader's active wrapping mode;
it is zero when an explicit outer object disables object wrapping. Implicit
containers remain open at EOF and are excluded from `Closer`, while `Start` still
refers to the original source.

`completion.Complete(Config{}, results)` adds matching replacements, selecting
field names or values from the parsed completion state. Field suggestions use
the completion's token and pointer, omitting keys already read in that object.
It uses `json` tags for names and `help` tags for descriptions, skips ignored and
unexported fields, and follows pointers into nested structs, arrays, slices, and
map values. Results map complete relaxed JSON field names to descriptions;
quoted names stay quoted, and field names include a following colon. For example,
a field tagged `json:"name" help:"Service name"` produces
`map["name:"] = "Service name"`. Comments produce no suggestions. Create the
completion with `MakeCompletion(reader)`.

String-keyed maps suggest `_key_:` for an empty key prefix, or the
entered key followed by `:`. Existing keys are omitted and notes show each key's
JSON Pointer. This keeps closing an empty map from being its only suggestion.

After a completed object or array, `Complete` returns an empty replacement so
the shell can finish the current word, alongside any remaining outer closers.
It suggests remaining closing containers with the description `close` at a new
token position or after another closing container. Active scalar and string
prefixes receive value suggestions only: `{enabled:t` suggests `true`, while
`{enabled:true ` can suggest `}`. The initial parse records whether the innermost
container can close, allowing all outer containers to close without parsing the
input again or requiring it to match the suggested Go type.
Optional `jsontext.Options` arguments to `MakeCompletion` configure parsing;
duplicate names are allowed by default for completion.

At value positions, `Complete` follows the full JSON Pointer and suggests `{}`
for structs and maps, `{field:` for struct fields, `[` for arrays and slices,
`true` and `false` for booleans, and `0` for numbers.
Struct fields use `help` tags as descriptions; other suggestions use
the JSON Pointer. Empty value prefixes offer an empty replacement alongside
`_str_` for strings or `0` for numbers. Nonempty string and number prefixes offer
themselves, preventing unrelated shell completions.
Pointers, maps, slices, and interfaces also offer `null`; empty interfaces offer
every JSON value kind. Byte slices use `""` for their base64 string representation,
while nonempty byte arrays and text-unmarshaled types offer an opening `"`.
Other suggestions match the current token prefix.

Options from both standard packages work directly, including indentation,
deterministic map ordering, custom marshalers/unmarshalers, unknown-member
rejection, and duplicate-name handling. Go struct tags and JSON customization
methods retain their v2 behavior. Custom methods see and produce standard JSON;
the relaxed syntax is handled at the outer boundary.

Duplicate names and invalid UTF-8 are rejected by default, as in v2. The reader
only converts relaxed syntax; `jsontext` validates the normalized JSON. Decoder
error byte offsets are translated using the input and output byte counts at the
previous token's start, accounting for inserted commas and quotes and removed
comments and whitespace before that token.

Run `go test ./...` to check the package and examples. Fuzz targets cover strict
JSON compatibility, optional commas, and relaxed round trips.
