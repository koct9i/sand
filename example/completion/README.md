# Completion examples

With `sand` on your `PATH`, load Bash completion with:

```bash
source <(sand completion bash)
```

Explore `sand example completion --help`, then press Tab after flag names or values.
The command prints the selected values as JSON:

```bash
sand example completion --format json --port 8081 --delay 5s --size 2MiB --ratio 0.5
sand example completion --enabled=true --color=false --tag blue,red --fields=id,name
sand example completion arguments staging 3 api worker
sand example completion json name:demo 'server:{host:localhost port:8080}'
```

- String, integer, boolean, duration, and floating-point flags suggest values.
- `--size` suggests size suffixes with byte totals and prints its parsed byte count as `size`.
- `--color` also has a `--no-color` inverse.
- `--tag` and `--fields` complete comma-separated lists and omit selected entries.
- `--fields` accepts a bare flag, which selects the default `id,name` fields.
- `arguments` completes a string, an integer, and any number of service names.
- `json` completes field names and nested fields across multiple shell arguments.

To inspect suggestions directly without installing a shell script:

```bash
go run . completion --complete bash -- example completion --format ""
go run . completion --complete fish -- example completion --size "2M"
go run . completion --complete bash -- example completion --fields = "id,n"
go run . completion --complete bash -- example completion arguments staging 3 ""
go run . completion --complete bash -- example completion json "server:{" "ho"
```

Use `ValueCompletion.Suggestions`, available as `Suggestions` on flags and arguments,
for fixed values such as `[]cli.Suggestion{{Text: "json", Note: "Structured JSON"}}`.
Completion filters these values by the current input prefix.

Callbacks handle comma-separated values, repeated arguments, and relaxed JSON.
They read `Completion.Input` and pass completed values and notes to `Completion.Suggest`.
Repeated arguments can inspect `Completion.Arguments` to omit earlier selections.
`MakeCompletion` sorts `Completion.Output` alphabetically while preserving the order
of suggestions with the same text. `Reply` applies `AddPrefix` and then `CutPrefix`
for inline `--flag=value` syntax and the shell's replacement span.

When Bash needs to prevent automatic replacement, its fixup includes the current
value to preserve the entered text. `Reply` applies the same prefix rules to every
entry. Candidate values and both prefixes are preserved.
