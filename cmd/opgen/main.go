// Command opgen generates a service's client surface from its OpenAPI document.
//
// The document is what a zip service writes with its own verb:
//
//	service openapi openapi.json
//	opgen -spec openapi.json -out sdk
//
// Two commands, no linking, so the generator reaches a repository it does not
// build. What it writes from a document is the command tree and the Rust and
// C++ clients. The Go SDK is not here: it speaks the ZAP wire, which the
// document does not describe, so it is generated in the service's own process
// through [github.com/luxfi/opgen.Emit].
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/luxfi/opgen"
	"github.com/zap-proto/zip"
)

func main() {
	spec := flag.String("spec", "", "the OpenAPI document to read")
	out := flag.String("out", "", "the directory to write into")
	name := flag.String("name", "", "what to call the generated crate and namespace (default: the service's own name)")
	only := flag.String("only", "", "a comma separated subset of "+names(opgen.All))
	flag.Parse()

	if *spec == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	doc, err := os.ReadFile(*spec)
	if err != nil {
		fail(err)
	}
	o := opgen.Options{Dir: *out, Name: *name}
	if *only != "" {
		for _, p := range strings.Split(*only, ",") {
			o.Only = append(o.Only, zip.Projection(strings.TrimSpace(p)))
		}
	}
	r, err := opgen.EmitSpec(doc, o)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d ops, %d types\n", r.Ops, r.Types)
	for _, f := range r.Files {
		fmt.Println(f)
	}
}

func names(ps []zip.Projection) string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return strings.Join(out, ", ")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
