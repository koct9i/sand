package cli_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/koct9i/sand/cli"
)

type jsonrServer struct {
	Host  string `json:"host" help:"Server host"`
	Ports []int  `json:"ports" help:"Listening ports"`
}

type jsonrConfig struct {
	Name    string                 `json:"name" help:"Service name"`
	Enabled bool                   `json:"enabled" help:"Enable service"`
	Server  jsonrServer            `json:"server" help:"Server settings"`
	Servers []jsonrServer          `json:"servers"`
	ByName  map[string]jsonrServer `json:"byName"`
	Spaced  string                 `json:"two words" help:"Quoted field"`
	Hidden  string                 `json:"-"`
}

func TestJSONRArgsParse(t *testing.T) {
	want := jsonrConfig{Name: "example", Enabled: true, Server: jsonrServer{Host: "localhost", Ports: []int{8080, 8081}}}
	for _, args := range [][]string{
		{"name:example", "enabled:true", "server:{", "host:localhost", "ports:[8080", "8081]", "}"},
		{"name:example\nenabled:true\nserver:{host:localhost ports:[8080 8081]}"},
		{"{name:example enabled:true server:{host:localhost ports:[8080,8081,]}}"},
		{"# configuration\n{", "name:example // service name", "enabled:true", "server:{host:localhost ports:[8080 8081]}", "}"},
		{"name:'example' # service name", "enabled:true", "server:{host:'localhost' ports:[8080 8081]}"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var got jsonrConfig
			a := cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Destination: &got, Max: -1}
			cmd := cli.Command{
				Name:      "configure",
				Writer:    io.Discard,
				ErrWriter: io.Discard,
				Arguments: []cli.Argument{&a},
			}
			if err := cmd.Run(t.Context(), append([]string{"configure"}, args...)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(a.Get(), want) {
				t.Fatalf("Destination = %#v, Get = %#v, want %#v", got, a.Get(), want)
			}
		})
	}
}

func TestJSONRArgsDefaultsAndLimits(t *testing.T) {
	initial := jsonrConfig{Name: "default", Enabled: true}
	var got jsonrConfig
	a := cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Value: initial, Destination: &got, Max: 1}
	if !a.HasName("config") || a.HasName("other") || a.Usage() != "[config]" || !reflect.DeepEqual(a.Get(), initial) {
		t.Fatalf("unexpected argument metadata: usage %q, value %#v", a.Usage(), a.Get())
	}
	if rest, err := a.Parse(nil); err != nil || len(rest) != 0 || !reflect.DeepEqual(got, initial) {
		t.Fatalf("empty input: rest %v, error %v, value %#v", rest, err, got)
	}
	if rest, err := a.Parse([]string{"name:custom", "remaining"}); err != nil || !reflect.DeepEqual(rest, []string{"remaining"}) || got.Name != "custom" || !got.Enabled {
		t.Fatalf("bounded input: rest %v, error %v, value %#v", rest, err, got)
	}
	if _, err := a.Parse(nil); err != nil || !reflect.DeepEqual(got, initial) {
		t.Fatalf("repeated parse retained previous input: value %#v, error %v", got, err)
	}
	a.UsageText = "JSON configuration"
	if a.Usage() != a.UsageText {
		t.Fatalf("Usage = %q, want %q", a.Usage(), a.UsageText)
	}
}

func TestJSONRArgsRejectInvalidInput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		min     int
		max     int
		options []json.Options
	}{
		{name: "syntax", args: []string{"server:{"}, max: -1},
		{name: "type", args: []string{"enabled:hello"}, max: -1},
		{name: "duplicate", args: []string{"name:first name:second"}, max: -1},
		{name: "unknown", args: []string{"missing:1"}, max: -1, options: []json.Options{json.RejectUnknownMembers(true)}},
		{name: "missing", min: 1, max: -1},
		{name: "zero max"},
		{name: "negative max", max: -2},
		{name: "negative min", min: -1, max: -1},
		{name: "min greater than max", min: 2, max: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonrConfig{Name: "unchanged"}
			a := cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Destination: &got, Min: tt.min, Max: tt.max, Options: tt.options}
			rest, err := a.Parse(tt.args)
			if err == nil || !strings.Contains(err.Error(), "config") {
				t.Fatalf("Parse error = %v, want an error naming config", err)
			}
			if !reflect.DeepEqual(rest, tt.args) || got.Name != "unchanged" {
				t.Fatalf("invalid input consumed arguments or changed destination: rest %v, value %#v", rest, got)
			}
		})
	}
}

func TestJSONRArgsCompletion(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want map[string]string
	}{
		{[]string{""}, map[string]string{
			"name:":          "Service name",
			"enabled:":       "Enable service",
			"server:":        "Server settings",
			"servers:":       "",
			"byName:":        "",
			"\"two words\":": "Quoted field",
		}},
		{[]string{"na"}, map[string]string{"name:": "Service name"}},
		{[]string{"{na"}, map[string]string{"{name:": "Service name"}},
		{[]string{"byName:"}, map[string]string{"byName:{": "/byName", "byName:null": "/byName"}},
		{[]string{"byName:", ""}, map[string]string{"{": "/byName", "null": "/byName"}},
		{[]string{"{byName:"}, map[string]string{"{byName:{": "/byName", "{byName:null": "/byName"}},
		{[]string{"byName: /* done */"}, map[string]string{"byName: /* done */{": "/byName", "byName: /* done */null": "/byName"}},
		{[]string{"byName:n"}, map[string]string{"byName:null": "/byName"}},
		{[]string{"byName:{primary:"}, map[string]string{"byName:{primary:{": "/byName/primary"}},
		{[]string{"byName:{'':"}, map[string]string{"byName:{'':{": "/byName/"}},
		{[]string{"byName:{"}, map[string]string{"byName:{}": ""}},
		{[]string{"server:"}, map[string]string{"server:{": "/server"}},
		{[]string{"servers:"}, map[string]string{"servers:[": "/servers", "servers:null": "/servers"}},
		{[]string{"servers:["}, map[string]string{"servers:[{": "/servers/0", "servers:[]": ""}},
		{[]string{"server:{ports:"}, map[string]string{"server:{ports:[": "/server/ports", "server:{ports:null": "/server/ports"}},
		{[]string{"server:{ports:["}, map[string]string{"server:{ports:[0": "/server/ports/0", "server:{ports:[1": "/server/ports/0", "server:{ports:[]}": ""}},
		{[]string{"enabled:"}, map[string]string{"enabled:true": "/enabled", "enabled:false": "/enabled"}},
		{[]string{"enabled:t"}, map[string]string{"enabled:true": "/enabled"}},
		{[]string{"enabled:f"}, map[string]string{"enabled:false": "/enabled"}},
		{[]string{"name:"}, map[string]string{"name:_": "/name", "name:string": "/name"}},
		{[]string{"name:hello"}, map[string]string{"name:hello": "/name", "name:hello,": "/name"}},
		{[]string{"name:", "hello"}, map[string]string{"hello": "/name", "hello,": "/name"}},
		{[]string{"name:example en"}, map[string]string{"name:example enabled:": "Enable service"}},
		{[]string{`name:"x"en`}, map[string]string{`name:"x"enabled:`: "Enable service"}},
		{[]string{`name:'x'en`}, map[string]string{`name:'x'enabled:`: "Enable service"}},
		{[]string{`name:'x'/* done */en`}, map[string]string{`name:'x'/* done */enabled:`: "Enable service"}},
		{[]string{"server:{ho"}, map[string]string{"server:{host:": "Server host"}},
		{[]string{"server:{", "ho"}, map[string]string{"host:": "Server host"}},
		{[]string{"servers:[{", "ho"}, map[string]string{"host:": "Server host"}},
		{[]string{"byName:{primary:{ho"}, map[string]string{"byName:{primary:{host:": "Server host"}},
		{[]string{"server:{\"\\u0068o"}, map[string]string{"server:{host:": "Server host"}},
		{[]string{"'two w"}, map[string]string{"\"two words\":": "Quoted field"}},
		{[]string{"/* comment */ na"}, map[string]string{"/* comment */ name:": "Service name"}},
		{[]string{"name:example # comment", "en"}, map[string]string{"enabled:": "Enable service"}},
		{[]string{"server:{host:localhost"}, map[string]string{"server:{host:localhost": "/server/host", "server:{host:localhost,": "/server/host", "server:{host:localhost}": ""}},
		{[]string{"name:'hel"}, map[string]string{"name:hel": "/name", "name:hel,": "/name", "name:'hel'": ""}},
		{[]string{"# comment"}, map[string]string{}},
		{[]string{"// comment"}, map[string]string{}},
		{[]string{"name:example # comment"}, map[string]string{}},
		{[]string{"name:example // comment"}, map[string]string{}},
		{[]string{"name:example /* unfinished"}, map[string]string{}},
		{[]string{"/* unfinished"}, map[string]string{}},
		{[]string{"{name:example}"}, map[string]string{}},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			destination := jsonrConfig{Name: "unchanged"}
			a := cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Destination: &destination, Min: 10, Max: -1}
			c := &cli.Completion{
				Arguments: tt.args, Input: tt.args[len(tt.args)-1],
			}
			a.Complete(t.Context(), c)
			got := make(map[string]string, len(c.Output))
			for _, value := range c.Output {
				got[value.Text] = value.Note
			}
			if len(c.Output) != len(tt.want) || !reflect.DeepEqual(got, tt.want) || destination.Name != "unchanged" {
				t.Fatalf("Complete = %v, want %v; destination %#v", c.Output, tt.want, destination)
			}
		})
	}
}

func TestJSONRArgsShellCompletion(t *testing.T) {
	var output bytes.Buffer
	destination := jsonrConfig{Name: "unchanged"}
	a := cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Destination: &destination, Max: -1}
	root := cli.Command{
		Name:           "sand",
		Writer:         &output,
		ErrWriter:      io.Discard,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Commands: []*cli.Command{
			cli.CompletionCommand(),
			{
				Name:      "configure",
				Arguments: []cli.Argument{&a},
			},
		},
	}
	for _, shell := range []string{"bash", "fish", "zsh"} {
		for _, args := range [][]string{{""}, {"server:{", "ho"}, {"na"}} {
			output.Reset()
			command := append([]string{"sand", "completion", "--complete", shell, "--", "configure"}, args...)
			err := root.Run(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			want, help := "name:", "Service name"
			if len(args) > 1 {
				want, help = "host:", "Server host"
			}
			switch shell {
			case "fish":
				want += "\t" + help
			case "zsh":
				want = strings.ReplaceAll(want, ":", "\\:") + ":" + help
			}
			want += "\n"
			if args[0] == "" {
				switch shell {
				case "bash":
					want = "\"two words\":   (Quoted field)\nbyName:\nenabled:       (Enable service)\nname:          (Service name)\nserver:        (Server settings)\nservers:\n"
				case "fish":
					want = "\"two words\":\tQuoted field\nbyName:\t\nenabled:\tEnable service\nname:\tService name\nserver:\tServer settings\nservers:\t\n"
				case "zsh":
					want = "\"two words\"\\::Quoted field\nbyName\\::\nenabled\\::Enable service\nname\\::Service name\nserver\\::Server settings\nservers\\::\n"
				}
			}
			if output.String() != want || destination.Name != "unchanged" {
				t.Fatalf("%s completion of %v: output %q, destination %#v", shell, args, output.String(), destination)
			}
		}
	}
}

func TestJSONRArgsValueShellCompletion(t *testing.T) {
	root := &cli.Command{
		Name:      "configure",
		Arguments: []cli.Argument{&cli.RelaxedJSONArguments[jsonrConfig]{Name: "config", Max: -1}},
	}
	for _, tt := range []struct {
		shell, input, want string
	}{
		{"bash", "byName:", "byName:null   (/byName)\nbyName:{      (/byName)\n"},
		{"fish", "byName:", "byName:null\t/byName\nbyName:{\t/byName\n"},
		{"zsh", "byName:", "byName\\:null:/byName\nbyName\\:{:/byName\n"},
		{"bash", "enabled:", "enabled:false   (/enabled)\nenabled:true    (/enabled)\n"},
		{"fish", "enabled:", "enabled:false\t/enabled\nenabled:true\t/enabled\n"},
		{"zsh", "enabled:", "enabled\\:false:/enabled\nenabled\\:true:/enabled\n"},
	} {
		t.Run(tt.shell+"/"+tt.input, func(t *testing.T) {
			c := cli.MakeCompletion(t.Context(), root, tt.shell, []string{tt.input}, nil)
			var output bytes.Buffer
			if err := c.Reply(&output); err != nil {
				t.Fatal(err)
			}
			if output.String() != tt.want {
				t.Fatalf("Reply = %q, want %q", output.String(), tt.want)
			}
		})
	}
}

func ExampleRelaxedJSONArguments() {
	type Config struct {
		Name string `json:"name" help:"Service name"`
		Port int    `json:"port" help:"Listening port"`
	}
	var config Config
	arg := cli.RelaxedJSONArguments[Config]{Name: "config", Destination: &config, Max: -1}
	if _, err := arg.Parse([]string{"name:example", "port:8080"}); err != nil {
		panic(err)
	}
	fmt.Println(config.Name, config.Port)
	// Output: example 8080
}
