package completion

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/koct9i/sand/cli"
)

const (
	defaultFields = "id,name"
	jsonFormat    = "json"
)

// NewCommand demonstrates suggestions and completion callbacks for flags and positional arguments.
func NewCommand() *cli.Command {
	boolean := []cli.Suggestion{{Text: "false", Note: "Disabled"}, {Text: "true", Note: "Enabled"}}
	return &cli.Command{
		Name:  "completion",
		Usage: "try flag and argument completion",
		Description: `Running an example prints its parsed values as JSON.

Examples:
  sand example completion --format json --port 8081 --size 2MiB --tag blue,red
  sand example completion --enabled=true --color=false --delay 5s
  sand example completion --fields=id,name
  sand example completion arguments staging 3 api worker
  sand example completion json name:demo 'server:{host:localhost port:8080}'
  sand example completion json 'children:{api:{children:{worker:{enabled:true}}}}'`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "format", Aliases: []string{"f"}, Value: jsonFormat, Usage: "Example format value",
				Suggestions: []cli.Suggestion{
					{Text: jsonFormat, Note: "Structured JSON"},
					{Text: "yaml", Note: "YAML document"},
					{Text: "text", Note: "Plain text"},
				},
			},
			&cli.IntFlag{
				Name: "port", Aliases: []string{"p"}, Value: 8080, Usage: "Example port number",
				Suggestions: []cli.Suggestion{
					{Text: "8080", Note: "Development HTTP"},
					{Text: "8081", Note: "Alternate HTTP"},
					{Text: "9090", Note: "Metrics"},
				},
			},
			&cli.BoolFlag{Name: "enabled", Usage: "Complete an explicit boolean value with --enabled=", Suggestions: boolean},
			&cli.BoolWithInverseFlag{Name: "color", Value: true, Usage: "Boolean with a --no-color inverse", Suggestions: boolean},
			&cli.DurationFlag{
				Name: "delay", Value: time.Second, Usage: "Example duration",
				Suggestions: []cli.Suggestion{
					{Text: "100ms", Note: "Quick"},
					{Text: "1s", Note: "Normal"},
					{Text: "5s", Note: "Slow"},
				},
			},
			&cli.SizeFlag{
				Name: "size", Value: 1 << 20, Usage: "Example size in bytes, accepting suffixes such as MB and MiB",
			},
			&cli.Float64Flag{
				Name: "ratio", Value: 1, Usage: "Example floating-point value",
				Suggestions: []cli.Suggestion{
					{Text: "0.25", Note: "Quarter"},
					{Text: "0.5", Note: "Half"},
					{Text: "1", Note: "Whole"},
				},
			},
			&cli.StringSliceFlag{
				Name: "tag", Usage: "Repeat the flag or complete a comma-separated list",
				Completion: commaSeparated(map[string]string{"blue": "Blue tag", "green": "Green tag", "red": "Red tag"}),
			},
			&struct {
				cli.StringFlag
				cli.ValueOptional
			}{
				Name: "fields", Value: defaultFields, Usage: "Optional field list: --fields or --fields=id,name",
				Completion: commaSeparated(map[string]string{"id": "Identifier", "name": "Display name", "status": "Current status"}),
			},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			fields := command.String("fields")
			if fields == "true" {
				fields = defaultFields
			}
			return writeJSON(command.Writer, map[string]any{
				"format":  command.String("format"),
				"port":    command.Int("port"),
				"enabled": command.Bool("enabled"),
				"color":   command.Bool("color"),
				"delay":   command.Duration("delay").String(),
				"size":    command.Uint64("size"),
				"ratio":   command.Float64("ratio"),
				"tags":    command.StringSlice("tag"),
				"fields":  fields,
			})
		},
		Commands: []*cli.Command{argumentsCommand(), jsonCommand()},
	}
}

func argumentsCommand() *cli.Command {
	return &cli.Command{
		Name:  "arguments",
		Usage: "complete a string, an integer, and repeated service names",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name: "environment", Value: "development",
				Suggestions: []cli.Suggestion{
					{Text: "development", Note: "Local development"},
					{Text: "staging", Note: "Pre-production"},
					{Text: "production", Note: "Live environment"},
				},
			},
			&cli.IntArg{
				Name: "replicas", Value: 1,
				Suggestions: []cli.Suggestion{
					{Text: "1", Note: "One replica"},
					{Text: "2", Note: "Two replicas"},
					{Text: "3", Note: "Three replicas"},
					{Text: "5", Note: "Five replicas"},
				},
			},
			&cli.StringArgs{
				Name: "services", Max: -1,
				Completion: choices(map[string]string{"api": "HTTP API", "web": "Web frontend", "worker": "Background jobs"}),
			},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			return writeJSON(command.Writer, map[string]any{
				"environment": command.StringArg("environment"),
				"replicas":    command.IntArg("replicas"),
				"services":    command.StringArgs("services"),
			})
		},
	}
}

type serverConfig struct {
	Host string `json:"host" help:"Server host"`
	Port int    `json:"port" help:"Server port"`
}

type serviceConfig struct {
	Name     string                   `json:"name" help:"Service name"`
	Enabled  bool                     `json:"enabled" help:"Enable the service"`
	Server   serverConfig             `json:"server" help:"Server settings"`
	Tags     []string                 `json:"tags" help:"Service tags"`
	Children map[string]serviceConfig `json:"children,omitempty" help:"Nested services"`
}

func jsonCommand() *cli.Command {
	var config serviceConfig
	return &cli.Command{
		Name:  "json",
		Usage: "complete relaxed JSON fields, including recursive service dictionaries",
		Arguments: []cli.Argument{
			&cli.RelaxedJSONArguments[serviceConfig]{
				Name: "config", 
				Destination: &config,
				Options: []json.Options{json.RejectUnknownMembers(true)},
			},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			return writeJSON(command.Writer, config)
		},
	}
}

func choices(candidates map[string]string) cli.CompleteFunc {
	return func(_ context.Context, completion *cli.Completion) {
		selected := completion.Arguments
		if len(selected) > 0 {
			selected = selected[:len(selected)-1]
		}
		for value, usage := range candidates {
			if strings.HasPrefix(value, completion.Input) && !slices.Contains(selected, value) {
				completion.Suggest(value, usage)
			}
		}
	}
}

func commaSeparated(candidates map[string]string) cli.CompleteFunc {
	return func(_ context.Context, completion *cli.Completion) {
		value := completion.Input
		comma := strings.LastIndex(value, ",") + 1
		base, prefix := value[:comma], value[comma:]
		selected := strings.Split(base, ",")
		for candidate, usage := range candidates {
			if strings.HasPrefix(candidate, prefix) && !slices.Contains(selected, candidate) {
				completion.Suggest(base+candidate, usage)
			}
		}
	}
}

func writeJSON(writer io.Writer, value any) error {
	if err := json.MarshalWrite(writer, value, json.Deterministic(true), jsontext.WithIndent("  ")); err != nil {
		return err
	}
	_, err := fmt.Fprintln(writer)
	return err
}
