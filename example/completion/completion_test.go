package completion_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"reflect"
	"testing"

	"github.com/koct9i/sand/cli"
	"github.com/koct9i/sand/example/completion"
)

func TestCompletionExamples(t *testing.T) {
	for _, tt := range []struct {
		name  string
		shell string
		words []string
		want  map[string]string
	}{
		{"subcommand", "fish", []string{"arg"}, map[string]string{"arguments": "complete a string, an integer, and repeated service names"}},
		{"flag name", "fish", []string{"--fo"}, map[string]string{"--format": "Example format value"}},
		{"string", "fish", []string{"--format", "j"}, map[string]string{"json": "Structured JSON"}},
		{"alias", "fish", []string{"-f", "y"}, map[string]string{"yaml": "YAML document"}},
		{"inline", "fish", []string{"--format=j"}, map[string]string{"json": "Structured JSON"}},
		{"bash word break", "bash", []string{"--format", "=", "j"}, map[string]string{"json": "Structured JSON"}},
		{"integer", "fish", []string{"--port", "80"}, map[string]string{"8080": "Development HTTP", "8081": "Alternate HTTP"}},
		{"boolean", "fish", []string{"--enabled=tr"}, map[string]string{"true": "Enabled"}},
		{"inverse boolean", "fish", []string{"--no-col"}, map[string]string{"--no-color": "Boolean with a --no-color inverse"}},
		{"duration", "fish", []string{"--delay", "1"}, map[string]string{"100ms": "Quick", "1s": "Normal"}},
		{"size", "fish", []string{"--size", "2M"}, map[string]string{
			"2MB":  "Megabytes (1000^2 bytes); 2000000 bytes total",
			"2MiB": "Mebibytes (1024^2 bytes); 2097152 bytes total",
		}},
		{"float", "fish", []string{"--ratio", "0."}, map[string]string{"0.25": "Quarter", "0.5": "Half"}},
		{"slice", "fish", []string{"--tag", "blue,r"}, map[string]string{"blue,red": "Red tag"}},
		{"optional value", "fish", []string{"--fields=id,n"}, map[string]string{"id,name": "Display name"}},
		{"bare optional flag", "fish", []string{"--fields", "arguments", "pr"}, map[string]string{"production": "Live environment"}},
		{"string argument", "fish", []string{"arguments", "st"}, map[string]string{"staging": "Pre-production"}},
		{"integer argument", "fish", []string{"arguments", "staging", "3"}, map[string]string{"3": "Three replicas"}},
		{"repeated argument", "fish", []string{"arguments", "staging", "3", "a"}, map[string]string{"api": "HTTP API"}},
		{"omit selected arguments", "fish", []string{"arguments", "staging", "3", "api", ""}, map[string]string{"web": "Web frontend", "worker": "Background jobs"}},
		{"json field", "fish", []string{"json", "ser"}, map[string]string{"server:": "Server settings"}},
		{"nested json field", "fish", []string{"json", "server:{", "ho"}, map[string]string{"host:": "Server host"}},
		{"omit used json field", "fish", []string{"json", "server:{host:localhost", ""}, map[string]string{"port:": "Server port", "}": "close"}},
		{"json object", "fish", []string{"json", "server:"}, map[string]string{"server:{}": "/server", "server:{host:": "Server host", "server:{port:": "Server port"}},
		{"bash json object", "bash", []string{"json", "server", ":", ""}, map[string]string{"{}": "/server", "{host:": "Server host", "{port:": "Server port"}},
		{"json boolean", "fish", []string{"json", "enabled:"}, map[string]string{"enabled:true": "/enabled", "enabled:false": "/enabled"}},
		{"json slice", "fish", []string{"json", "tags:"}, map[string]string{"tags:[": "/tags", "tags:null": "/tags"}},
		{"finished json slice", "bash", []string{"json", "tags", ":", "[]"}, map[string]string{"[]": ""}},
		{"json string prefix", "fish", []string{"json", "server:{host:localhost"}, map[string]string{"server:{host:localhost": "/server/host"}},
		{"recursive json dict", "fish", []string{"json", "children:"}, map[string]string{"children:{}": "/children", "children:null": "/children"}},
		{"empty recursive json dict", "bash", []string{"json", "children", ":", "{"}, map[string]string{"{_key_:": "/children/_key_", "{}": "close"}},
		{"recursive json dict key", "bash", []string{"json", "children", ":", "{api"}, map[string]string{"{api:": "/children/api"}},
		{"nested recursive json dict keys", "fish", []string{"json", "children:{api:{children:{"}, map[string]string{
			"children:{api:{children:{_key_:": "/children/api/children/_key_",
			"children:{api:{children:{}}}":    "close",
		}},
		{"recursive json dict field", "fish", []string{"json", "children:{api:{", "ch"}, map[string]string{"children:": "Nested services"}},
		{"recursive json field", "bash", []string{"json", "children:{api:{children:{worker:{", "en"}, map[string]string{"enabled:": "Enable the service"}},
		{"escaped recursive json field", "bash", []string{"json", `children\:\{api\:\{children\:\{worker\:\{`, "en"}, map[string]string{"enabled:": "Enable the service"}},
		{"escaped recursive word breaks", "bash", []string{"json", "children", ":", `\{api`, ":", `\{en`}, map[string]string{`\{enabled:`: "Enable the service"}},
		{"recursive json value", "fish", []string{"json", "children:{api:{children:{worker:{enabled:"}, map[string]string{
			"children:{api:{children:{worker:{enabled:true":  "/children/api/children/worker/enabled",
			"children:{api:{children:{worker:{enabled:false": "/children/api/children/worker/enabled",
		}},
		{"recursive json boolean prefix", "fish", []string{"json", "children:{api:{children:{worker:{enabled:t"}, map[string]string{
			"children:{api:{children:{worker:{enabled:true": "/children/api/children/worker/enabled",
		}},
		{"recursive json closer at next word", "fish", []string{"json", "children:{api:{children:{worker:{enabled:true", ""}, map[string]string{
			"children:": "Nested services", "name:": "Service name", "server:": "Server settings", "tags:": "Service tags", "}}}}": "close",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := &cli.Command{
				Name: "sand",
				Commands: []*cli.Command{{
					Name: "example", Commands: []*cli.Command{completion.NewCommand()},
				}},
			}
			words := append([]string{"example", "completion"}, tt.words...)
			got := cli.MakeCompletion(t.Context(), root, tt.shell, words, nil)
			values := make(map[string]string, len(got.Output))
			for _, value := range got.Output {
				values[value.Text] = value.Note
			}
			if len(got.Output) != len(tt.want) || !reflect.DeepEqual(values, tt.want) {
				t.Fatalf("completion of %q = %v, want %v", words, got.Output, tt.want)
			}
		})
	}
}

func TestExampleCommands(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "flags",
			args: []string{"--format", "yaml", "--port", "8081", "--enabled", "--no-color", "--delay", "5s", "--size", "2MiB", "--ratio", "0.5", "--tag", "blue,red", "--fields=id,status"},
			want: map[string]any{
				"format": "yaml", "port": float64(8081), "enabled": true, "color": false,
				"delay": "5s", "size": float64(2 << 20), "ratio": 0.5, "tags": []any{"blue", "red"}, "fields": "id,status",
			},
		},
		{name: "optional value", args: []string{"--fields"}, want: map[string]any{"fields": "id,name"}},
		{
			name: "arguments", args: []string{"arguments", "staging", "3", "api", "worker"},
			want: map[string]any{"environment": "staging", "replicas": float64(3), "services": []any{"api", "worker"}},
		},
		{
			name: "json", args: []string{"json", "name:demo", "enabled:true", "server:{host:localhost port:8080}"},
			want: map[string]any{"name": "demo", "enabled": true, "server": map[string]any{"host": "localhost", "port": float64(8080)}},
		},
		{
			name: "recursive json", args: []string{"json", "children:{api:{children:{worker:{enabled:true}}}}"},
			want: map[string]any{"children": map[string]any{
				"api": map[string]any{
					"name": "", "enabled": false, "server": map[string]any{"host": "", "port": float64(0)}, "tags": []any{},
					"children": map[string]any{
						"worker": map[string]any{
							"name": "", "enabled": true, "server": map[string]any{"host": "", "port": float64(0)}, "tags": []any{},
						},
					},
				},
			}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			root := &cli.Command{
				Name: "sand", Writer: &output, ErrWriter: io.Discard,
				ExitErrHandler: func(context.Context, *cli.Command, error) {},
				Commands:       []*cli.Command{completion.NewCommand()},
			}
			if err := root.Run(t.Context(), append([]string{"sand", "completion"}, tt.args...)); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("invalid JSON output %q: %v", output.String(), err)
			}
			for key, want := range tt.want {
				if !reflect.DeepEqual(got[key], want) {
					t.Errorf("%s = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}
