package jsonr_test

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"strings"

	"github.com/koct9i/sand/jsonr"
)

func ExampleUnmarshal() {
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
	fmt.Println(config.Name, config.Ports)
	// Output: example [8080 8081]
}

func ExampleMarshal() {
	b, err := jsonr.Marshal(struct {
		Name  string `json:"name"`
		Ports []int  `json:"ports"`
	}{Name: "example", Ports: []int{8080, 8081}})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	// Output: {name:example ports:[8080 8081]}
}

func ExampleRelaxedWriter() {
	var out bytes.Buffer
	w := jsonr.NewRelaxedWriter(&out)
	if err := json.MarshalWrite(w, struct {
		Name  string `json:"name"`
		Ports []int  `json:"ports"`
	}{Name: "example", Ports: []int{8080, 8081}}); err != nil {
		panic(err)
	}
	fmt.Println(out.String())
	// Output: {name:example ports:[8080 8081]}
}

func ExampleMakeCompletion() {
	last := jsonr.MakeCompletion(jsonr.NewRelaxedReader(strings.NewReader(`{users:[al`)))
	fmt.Printf("%q %q %q %d %t\n", last.Token, last.Pointer, last.Closer, last.Start, last.Value)
	last = jsonr.MakeCompletion(jsonr.NewRelaxedReader(strings.NewReader(`{name:demo en`)))
	fmt.Println(last.Keys)
	// Output:
	// "al" "/users/0" "]}" 8 true
	// [name]
}

func ExampleCompletion_Complete_fields() {
	type config struct {
		Name string `json:"name" help:"Service name"`
	}
	completions := map[string]string{}
	completion := jsonr.MakeCompletion(jsonr.NewRelaxedReader(strings.NewReader(`{na`)))
	completion.Complete(config{}, completions)
	fmt.Println(completions)
	// Output: map[name::Service name]
}

func ExampleCompletion_Complete_values() {
	type config struct {
		Enabled bool `json:"enabled"`
	}
	completions := map[string]string{}
	completion := jsonr.MakeCompletion(jsonr.NewRelaxedReader(strings.NewReader(`{enabled:t`)))
	completion.Complete(config{}, completions)
	fmt.Println(completions)
	// Output: map[true:/enabled]
}

func ExampleRelaxedReader_Wrap() {
	r := jsonr.NewRelaxedReader(strings.NewReader("1 2 # comment\n3,"))
	r.Wrap('[')
	var values []int
	if err := json.UnmarshalRead(r, &values); err != nil {
		panic(err)
	}
	fmt.Println(values)
	// Output: [1 2 3]
}
