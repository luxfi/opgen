// Package opgen generates a service's whole client surface from its typed ops.
//
// A zip app already projects one typed-op registry into an OpenAPI document, an
// MCP tool list, a command tree and a Go SDK. Each projection is reached by its
// own call, at its own moment, and nothing writes them together — so a fleet
// ends up with a docs site built in one pipeline, SDKs built in another and a
// tool manifest built by hand, which is three places for the same API to be
// described and two of them to be wrong.
//
// This writes all of them, once, from one app, and adds the two languages zip
// does not reach.
//
// # One source, two intermediate forms
//
// The typed ops in Go are the source. There is no second declaration anywhere,
// in any language.
//
// From that source the generator takes two intermediate forms, because there
// are two wires and neither one describes the other:
//
//	the OpenAPI document   the JSON edge. Complete for an HTTP client, which is
//	                       what the Rust, C++ and command-line legs are, and it
//	                       crosses a process boundary — so those legs can be
//	                       generated for a service this binary does not link.
//
//	the op registry        the ZAP wire, where a field is an offset and a width.
//	                       The document cannot describe it: JSON has no field
//	                       order, drops a `json:"-"` field the wire still gives
//	                       a slot to, and flattens an embedded struct the wire
//	                       nests. So the Go SDK, which speaks ZAP, is rendered
//	                       from the registry — by zip, which owns that wire.
//
// Everything downstream of the document reads the document's BYTES, not the Go
// value it was built from. That is what makes [Emit] and [EmitSpec] one
// pipeline rather than two implementations that have to be kept level.
//
// # What is not published
//
// A route registered through zip.Undeclared serves without being part of the
// contract — a retirement answering 410 is the case it exists for. zip's own
// extraction calls do not honour that: measured, an undeclared typed op appears
// in OpenAPISpec, in MCPTools and in Commands, while the live MCP door filters
// it out. Publishing from the raw calls would put a dead address in the docs
// site, in three SDKs and in the agent tool list. So every projection here is
// filtered through App.Declares first, and what this writes is the contract.
package opgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zap-proto/zip"
)

// The projections this adds to zip's own vocabulary. The verb is the value, so
// what a person types and what the code names are the same token.
const (
	MCP  zip.Projection = "mcp"
	CLI  zip.Projection = "cli"
	Go   zip.Projection = "go"
	Rust zip.Projection = "rust"
	Cpp  zip.Projection = "cpp"
)

// All is every projection, in the order they are written: the document first,
// because everything after it is derived from the document.
var All = []zip.Projection{zip.OpenAPI, MCP, CLI, Go, Rust, Cpp}

// Options says where to write and what to call it.
type Options struct {
	// Dir is the destination. It is created if it does not exist.
	Dir string

	// Name is what the generated package, crate and namespace are called. It
	// defaults to the app's own name, which is the right answer unless the app
	// is named something that is not an identifier.
	Name string

	// Only narrows the run to these projections. Empty means all of them.
	Only []zip.Projection
}

// Result is what a run wrote, and what it could not.
type Result struct {
	Dir   string
	Files []string // written, relative to Dir, sorted

	Ops   int // operations in the contract
	Types int // named types those operations move

	// Gaps are the operations a projection could NOT carry. An SDK that quietly
	// dropped one would compile and lie, so the run reports them and the caller
	// decides whether that is acceptable.
	Gaps []Gap
}

// Gap is one operation one projection cannot express.
type Gap struct {
	Where zip.Projection
	Op    string
	Why   string
}

// Emit writes the client surface for app into o.Dir.
//
// It builds the app first and refuses if it does not build. An app that fails
// to build projects as an EMPTY one — no ops, no routes — so without this a
// release pipeline writes an empty document, three empty SDKs and an empty tool
// list for a broken service, and exits 0.
func Emit(app *zip.App, o Options) (*Result, error) {
	if err := app.Build(); err != nil {
		return nil, fmt.Errorf("opgen: refusing to describe a program that does not build: %w", err)
	}
	if o.Dir == "" {
		return nil, fmt.Errorf("opgen: no destination directory")
	}
	name := o.Name
	if name == "" {
		name = snake(app.Declaration().Name)
	}
	if name == "" {
		return nil, fmt.Errorf("opgen: the app has no name and none was given")
	}
	want := chosen(o.Only)

	contract := published(app)
	doc, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("opgen: rendering the document: %w", err)
	}
	doc = append(doc, '\n')

	r := &Result{Dir: o.Dir}
	if want[zip.OpenAPI] {
		if err := write(r, o.Dir, "openapi.json", doc); err != nil {
			return nil, err
		}
	}
	if want[MCP] {
		tools := serves(app, app.MCPTools())
		b, err := json.MarshalIndent(tools, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("opgen: rendering the tool list: %w", err)
		}
		if err := write(r, o.Dir, "mcp.json", append(b, '\n')); err != nil {
			return nil, err
		}
	}
	if want[Go] {
		sdk, err := app.SDK(name)
		if err != nil {
			return nil, fmt.Errorf("opgen: the Go SDK: %w", err)
		}
		for _, g := range sdk.Gaps {
			r.Gaps = append(r.Gaps, Gap{Where: Go, Op: g.Op, Why: g.Cause})
		}
		if err := write(r, o.Dir, filepath.Join("go", name, name+".go"), sdk.Source); err != nil {
			return nil, err
		}
	}

	// Everything left is derived from the document's bytes, which is the same
	// input EmitSpec takes. One pipeline, reached two ways.
	rest, err := EmitSpec(doc, Options{Dir: o.Dir, Name: name, Only: o.Only})
	if err != nil {
		return nil, err
	}
	r.Files = append(r.Files, rest.Files...)
	r.Ops, r.Types = rest.Ops, rest.Types
	r.Gaps = append(r.Gaps, rest.Gaps...)
	sort.Strings(r.Files)
	sortGaps(r.Gaps)
	return r, nil
}

// EmitSpec writes the projections an OpenAPI document is enough to derive: the
// command tree and the Rust and C++ clients.
//
// This is the door for a service this binary does not link. The service writes
// its own document with `<binary> openapi <file>` — zip's own verb — and this
// reads it. The Go SDK is not reachable here and is not silently skipped: it
// speaks a wire the document does not describe, so asking for it from a
// document is refused.
func EmitSpec(doc []byte, o Options) (*Result, error) {
	if o.Dir == "" {
		return nil, fmt.Errorf("opgen: no destination directory")
	}
	s, err := Read(doc)
	if err != nil {
		return nil, err
	}
	name := o.Name
	if name == "" {
		name = snake(s.Name)
	}
	want := chosen(o.Only)
	r := &Result{Dir: o.Dir, Ops: len(s.Ops), Types: len(s.Types)}
	for _, op := range s.Ops {
		if len(op.Unbound) == 0 {
			continue
		}
		why := "the address carries " + strings.Join(op.Unbound, ", ") + ", which the input type has no field for"
		r.Gaps = append(r.Gaps, Gap{Where: Rust, Op: op.ID, Why: why}, Gap{Where: Cpp, Op: op.ID, Why: why})
	}
	sortGaps(r.Gaps)

	if want[CLI] {
		cmds, err := zip.CommandsFromSpec(doc)
		if err != nil {
			return nil, fmt.Errorf("opgen: the command tree: %w", err)
		}
		b, err := json.MarshalIndent(cmds, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("opgen: rendering the command tree: %w", err)
		}
		if err := write(r, o.Dir, "cli.json", append(b, '\n')); err != nil {
			return nil, err
		}
	}
	if want[Rust] {
		for path, body := range rust(s, name) {
			if err := write(r, o.Dir, filepath.Join("rust", path), body); err != nil {
				return nil, err
			}
		}
	}
	if want[Cpp] {
		for path, body := range cpp(s, name) {
			if err := write(r, o.Dir, filepath.Join("cpp", path), body); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(r.Files)
	return r, nil
}

func chosen(only []zip.Projection) map[zip.Projection]bool {
	want := map[zip.Projection]bool{}
	if len(only) == 0 {
		only = All
	}
	for _, p := range only {
		want[p] = true
	}
	return want
}

// write puts one file down atomically and records it.
func write(r *Result, dir, rel string, body []byte) error {
	dest := filepath.Join(dir, rel)
	if d := filepath.Dir(dest); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("opgen: %s: %w", rel, err)
		}
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("opgen: %s: %w", rel, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("opgen: %s: %w", rel, err)
	}
	r.Files = append(r.Files, rel)
	return nil
}

func sortGaps(g []Gap) {
	sort.Slice(g, func(i, j int) bool {
		if g[i].Where != g[j].Where {
			return g[i].Where < g[j].Where
		}
		return g[i].Op < g[j].Op
	})
}

// ---- what the app publishes -------------------------------------------------

// published is the app's OpenAPI document with the addresses it serves but does
// not declare removed, and the schemas only those addresses reached removed
// with them.
//
// The filter keys on the operationId and not on the path. A declaration spells
// a route the way the ROUTER does and a document spells it the way OpenAPI
// does, and the two do not invert: zip renders both "*" and "+" as
// "{wildcard1}", so reading the document's spelling back into a router pattern
// is a guess. Matching a wildcard route the wrong way dropped it from the
// contract silently, which is the failure mode a contract exists to prevent.
// An operationId is one token in both places — the same one the MCP tool, the
// command and the SDK method are named after.
func published(app *zip.App) map[string]any {
	// Re-read the document through JSON so what is filtered here is the same
	// value that gets written — OpenAPISpec hands back typed Go maps whose
	// shapes differ from the marshalled form.
	raw, err := json.Marshal(app.OpenAPISpec())
	if err != nil {
		return map[string]any{}
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return map[string]any{}
	}

	live := declares(app)
	paths, _ := doc["paths"].(map[string]any)
	for route, raw := range paths {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for method, op := range item {
			if !verb(strings.ToUpper(method)) {
				continue
			}
			decl, _ := op.(map[string]any)
			id, _ := decl["operationId"].(string)
			if !live[id] {
				delete(item, method)
			}
		}
		if len(item) == 0 {
			delete(paths, route)
		}
	}
	prune(doc)
	return doc
}

// declares is every operation the app publishes, by name.
func declares(app *zip.App) map[string]bool {
	live := map[string]bool{}
	for _, r := range app.Declaration().Routes {
		if r.Op != "" {
			live[r.Op] = true
		}
	}
	return live
}

// serves drops the tools whose op is not on a declared address. The live MCP
// door already does this; the extraction call does not, so a manifest built
// from it would offer an agent a tool the service will not answer.
func serves(app *zip.App, tools []map[string]any) []map[string]any {
	live := declares(app)
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		if name, _ := t["name"].(string); live[name] {
			out = append(out, t)
		}
	}
	return out
}

// prune drops every schema no remaining operation reaches.
func prune(doc map[string]any) {
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	if len(schemas) == 0 {
		return
	}
	keep := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if ref, ok := t["$ref"].(string); ok {
				name := refName(ref)
				if name != "" && !keep[name] {
					keep[name] = true
					walk(schemas[name])
				}
			}
			for k, child := range t {
				if k != "$ref" {
					walk(child)
				}
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(doc["paths"])
	for name := range schemas {
		if !keep[name] {
			delete(schemas, name)
		}
	}
	if len(schemas) == 0 {
		delete(components, "schemas")
	}
	if len(components) == 0 {
		delete(doc, "components")
	}
}
