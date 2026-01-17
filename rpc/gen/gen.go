package main

import (
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"slices"

	"k8s.io/gengo/v2"
	"k8s.io/gengo/v2/codetags"
	"k8s.io/gengo/v2/generator"
	"k8s.io/gengo/v2/namer"
	"k8s.io/gengo/v2/types"
)

type rpcGenerator struct {
	generator.GoGenerator
	outputPackage  *types.Package
	importsTracker namer.ImportTracker
}

func (g *rpcGenerator) Filter(c *generator.Context, t *types.Type) bool {
	if t.Kind != types.Interface {
		return false
	}
	tags := codetags.Extract("+sand:", t.CommentLines)
	_, ok := tags["rpc"]
	return ok
}

func isErrorType(t *types.Type) bool {
	if t == nil {
		return false
	}
	return t.Name.Name == "error"
}

func isContextType(t *types.Type) bool {
	if t == nil {
		return false
	}
	return t.Name.Package == "context" && t.Name.Name == "Context"
}

type Param struct {
	Name       string
	PublicName string
	Type       *types.Type
	Comma      string
	IsContext  bool
	IsError    bool
	IsVariadic bool
}

type Method struct {
	Name          string
	Context       Param
	Arguments     []Param
	Results       []Param
	Error         Param
	WithContext   bool
	WithArguments bool
	WithResults   bool
	WithError     bool
}

const rpcTemplate = `
// {{.Name}}Caller implements {{.Name}} via a Caller.
type {{.Name}}Caller struct {
	{{.Imports.Caller|raw}}
}
{{- range $method := .Methods}}
{{- if $method.WithArguments}}

type {{$method.Name}}Arg struct {
	{{- range $param := $method.Arguments}}
	{{- if $param.IsContext}}{{continue}}{{end}}
	{{$param.PublicName}} {{$param.Type|raw}}
	{{- end}}
}
{{- end}}
{{- if $method.WithResults}}

type {{$method.Name}}Res struct {
	{{- range $param := $method.Results}}
	{{- if $param.IsError}}{{continue}}{{end}}
	{{$param.PublicName}} {{$param.Type|raw}}
	{{- end}}
}
{{- end}}

func (s *{{$.Name}}Caller) {{$method.Name}} (
	{{- range $param := $method.Arguments}}
	{{- if $param.IsVariadic}}
	{{- $param.Comma}}{{$param.Name}} ...{{$param.Type.Elem}}
	{{- else}}
	{{- $param.Comma}}{{$param.Name}} {{$param.Type|raw}}
	{{- end }}
	{{- end -}}
) (
	{{- range $param := $method.Results}}
	{{- if $param.IsError}}{{continue}}{{end}}
	{{- $param.Comma}}{{$param.Name}} {{$param.Type|raw}}
	{{- end -}}
	{{- if $method.WithError}}
	{{- $method.Error.Comma}}{{$method.Error.Name}} {{$method.Error.Type}}
	{{- end -}}
) {
	{{- if $method.WithArguments}}
	arg_ := &{{$method.Name}}Arg{
		{{- range $param := $method.Arguments}}
		{{- if $param.IsContext}}{{continue}}{{end}}
		{{$param.PublicName}}: {{$param.Name}},
		{{- end}}
	}
	{{- end}}
	{{- if $method.WithResults}}
	var res_ {{$method.Name}}Res
	{{$method.Error.Name}}
	{{- if $method.WithError}} = {{else}} := {{end}}
	{{- else}}
	{{if $method.WithError}}return {{else}}{{$method.Error.Name}} := {{end}}
	{{- end -}}
	s.Caller.Call({{$method.Context.Name}}, "{{$method.Name}}",
	{{- if $method.WithArguments}}arg_{{else}}nil{{end}},
	{{- if $method.WithResults}}&res_{{else}}nil{{end}})
	{{- if not $method.WithError}}
	if {{$method.Error.Name}} != nil {
		panic({{$method.Error.Name}})
	}
	{{- end}}
	{{- if $method.WithResults}}
	{{"return "}}
	{{- range $param := $method.Results}}
	{{- if $param.IsError}}{{continue}}{{end}}
	{{- $param.Comma}}res_.{{$param.PublicName}}
	{{- end}}
	{{- if $method.WithError}}
	{{- $method.Error.Comma}}{{$method.Error.Name}}
	{{- end}}
	{{- end}}
}

{{- end}}

// {{.Name}}Handler implements Handler via a {{.Name}}.
type {{.Name}}Handler struct {
	{{.Name}}
}

{{- range $method := .Methods}}

func (h *{{$.Name}}Handler) {{$method.Name}} (ctx {{$.Imports.Context|raw}}, stream {{$.Imports.Stream|raw}}) error {
	{{- if $method.WithArguments}}
	var arg {{$method.Name}}Arg
	if err := stream.Recv(ctx, &arg); err != nil {
		return err
	}
	{{- end}}
	{{- if $method.WithResults}}
	var res {{$method.Name}}Res
	{{- end}}
	{{- if $method.WithError}}
	var err_ error
	{{- end}}
	{{range $param := $method.Results}}
	{{- if $param.IsError}}{{continue}}{{end}}
	{{- $param.Comma}}res.{{$param.PublicName}}
	{{- end}}
	{{- if $method.WithError}}
	{{- $method.Error.Comma}}err_
	{{- end}}
	{{- if or $method.WithResults $method.WithError}} = {{end -}}
	h.{{$.Name}}.{{$method.Name}}(
		{{- if $method.WithContext}}ctx{{end}}
		{{- range $param := $method.Arguments}}
		{{- if $param.IsContext}}{{continue}}{{end}}
		{{- $param.Comma}}arg.{{$param.PublicName}}
		{{- if $param.IsVariadic}}...{{end}}
		{{- end}})
	{{- if $method.WithError}}
	if err_ != nil {
		return err_
	}
	{{- end}}
	{{- if $method.WithResults}}
	if err := stream.Send(ctx, &res); err != nil {
		return err
	}
	{{- end}}
	return nil
}
{{- end}}

func (h *{{$.Name}}Handler) Methods() {{.Imports.Seq2|raw}}[string, {{.Imports.MethodFunc|raw}}] {
	return func(yield func(string, {{.Imports.MethodFunc|raw}}) bool) {
		{{- range $method := .Methods}}
		if !yield("{{$method.Name}}", h.{{$method.Name}}) {
			return
		}
		{{- end}}
	}
}

func (h *{{$.Name}}Handler) Serve(ctx {{.Imports.Context|raw}}, method string, stream {{.Imports.Stream|raw}}) error {
	switch method {
	{{- range $method := .Methods}}
	case "{{$method.Name}}":
		return h.{{$method.Name}}(ctx, stream)
	{{- end}}
	default:
		return {{.Imports.UnknownMethod|raw}}(method)
	}
}

`

func (g *rpcGenerator) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "{{", "}}")

	methods := make([]Method, 0, len(t.Methods))
	for _, mname := range slices.Sorted(maps.Keys(t.Methods)) {
		msign := *t.Methods[mname].Signature
		method := Method{
			Name: mname,
			Context: Param{
				Name:      "context.Background()",
				IsContext: true,
			},
			Error: Param{
				Name:    "err",
				IsError: true,
			},
		}
		for index, arg := range msign.Parameters {
			p := Param{
				Name: arg.Name,
				Type: arg.Type,
			}
			if index != 0 {
				p.Comma = ", "
			}
			if index == 0 && isContextType(p.Type) {
				if p.Name == "" {
					p.Name = "ctx"
				}
				p.IsContext = true
				method.Context = p
				method.WithContext = true
			}
			if index == len(msign.Parameters)-1 && msign.Variadic {
				if p.Name == "" {
					p.Name = "args"
				}
				p.IsVariadic = true
			}
			if p.Name == "" {
				p.Name = fmt.Sprintf("arg%v", index)
			}
			p.PublicName = namer.IC(p.Name)
			if !p.IsContext {
				method.WithArguments = true
			}
			method.Arguments = append(method.Arguments, p)
			g.importsTracker.AddType(c.Universe.Type(p.Type.Name))
		}
		for index, result := range msign.Results {
			p := Param{
				Name: result.Name,
				Type: result.Type,
			}
			if index != 0 {
				p.Comma = ", "
			}
			if index == len(msign.Results)-1 && isErrorType(result.Type) {
				if p.Name == "" {
					p.Name = "err"
				}
				p.IsError = true
				method.Error = p
				method.WithError = true
			}
			if p.Name == "" {
				p.Name = fmt.Sprintf("res%v", index)
			}
			p.PublicName = namer.IC(p.Name)
			if !p.IsError {
				method.WithResults = true
			}
			method.Results = append(method.Results, p)
			g.importsTracker.AddType(p.Type)
		}
		methods = append(methods, method)
	}

	importedTypes := []string{
		"context.Context",
		"iter.Seq2",
		"github.com/koct9i/sand/rpc.Caller",
		"github.com/koct9i/sand/rpc.Stream",
		"github.com/koct9i/sand/rpc.MethodFunc",
	}
	imports := generator.Args{}
	for _, t := range importedTypes {
		name := types.ParseFullyQualifiedName(t)
		tp := c.Universe.Type(name)
		g.importsTracker.AddType(tp)
		imports[name.Name] = tp
	}

	imports["UnknownMethod"] = c.Universe.Function(types.ParseFullyQualifiedName("github.com/koct9i/sand/rpc.UnknownMethod"))

	args := generator.Args{
		"Name":    t.Name.Name,
		"Methods": methods,
		"Imports": imports,
	}
	sw.Do(rpcTemplate, args)
	return sw.Error()
}

func (g *rpcGenerator) Imports(c *generator.Context) []string {
	return g.importsTracker.ImportLines()
}

func (g rpcGenerator) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage.Path, nil),
	}
}

func getTargets(c *generator.Context) []generator.Target {
	boilerplate, err := gengo.GoBoilerplate("", gengo.StdBuildTag, gengo.StdGeneratedBy)
	if err != nil {
		log.Fatalf("failed loading boilerplate: %v", err)
	}
	targets := []generator.Target{}
	for _, input := range c.Inputs {
		pkg := c.Universe[input]
		targets = append(targets, &generator.SimpleTarget{
			PkgName:       pkg.Name,
			PkgPath:       pkg.Path,
			PkgDir:        pkg.Dir,
			HeaderComment: boilerplate,
			FilterFunc: func(c *generator.Context, t *types.Type) bool {
				return t.Name.Package == pkg.Path
			},
			GeneratorsFunc: func(c *generator.Context) []generator.Generator {
				return []generator.Generator{
					&rpcGenerator{
						GoGenerator: generator.GoGenerator{
							OutputFilename: "rpc_generated.go",
						},
						outputPackage:  pkg,
						importsTracker: generator.NewImportTrackerForPackage(pkg.Path),
					},
				}
			},
		})
	}
	return targets
}

func main() {
	if len(os.Args) <= 1 {
		log.Fatal("Usage: //go:generate go run <gen> <package|pattern>...")
	}
	patterns := os.Args[1:]
	if err := gengo.Execute(
		namer.NameSystems{
			"public":  namer.NewPublicNamer(0),
			"private": namer.NewPrivateNamer(0),
			"raw":     namer.NewRawNamer("", nil),
		},
		"raw",
		getTargets,
		gengo.StdBuildTag,
		patterns,
	); err != nil {
		log.Fatalf("failed: %v", err)
	}
}
