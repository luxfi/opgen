// Command fixture writes the fixture service's client surface.
//
// What it produces is committed under example/, for two reasons. A reader can
// see exactly what this generates without running anything, and CI regenerates
// it and refuses a diff — so a change to an emitter shows up as a change to the
// code it emits, in the same review.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/luxfi/opgen"
	"github.com/luxfi/opgen/internal/fixture"
)

func main() {
	out := flag.String("out", "example", "the directory to write into")
	flag.Parse()

	r, err := opgen.Emit(fixture.App(), opgen.Options{Dir: *out})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, f := range r.Files {
		fmt.Println(f)
	}
	for _, g := range r.Gaps {
		fmt.Printf("gap %s: %s (%s)\n", g.Where, g.Op, g.Why)
	}
}
