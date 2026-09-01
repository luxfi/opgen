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
	MCP      zip.Projection = "mcp"
	Commands zip.Projection = "commands"
	Go       zip.Projection = "go"
	Rust     zip.Projection = "rust"
	Cpp      zip.Projection = "cpp"
)

// All is every projection, in the order they are written: the document first,
// because everything after it is derived from the document.
var All = []zip.Projection{zip.OpenAPI, MCP, Commands, Go, Rust, Cpp}

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
	// Field is where the value that cannot cross sits, "Type.field", when the
	// projection knows. A gap that says only "no codec" tells a reader that
	// something is wrong and not what to change.
	Field string
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
	name, err := called(o.Name, app.Declaration().Name)
	if err != nil {
		return nil, err
	}
	want := chosen(o.Only)

	contract := published(app)
	doc, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("opgen: rendering the document: %w", err)
	}
	doc = append(doc, '\n')

	r := &Result{Dir: o.Dir}
	out := sheet{}
	if want[zip.OpenAPI] {
		out["openapi.json"] = doc
	}
	if want[MCP] {
		b, err := json.MarshalIndent(serves(app, app.MCPTools()), "", "  ")
		if err != nil {
			return nil, fmt.Errorf("opgen: rendering the tool list: %w", err)
		}
		out["mcp.json"] = append(b, '\n')
	}
	if want[Go] {
		sdk, err := app.SDK(name)
		if err != nil {
			return nil, fmt.Errorf("opgen: the Go SDK: %w", err)
		}
		for _, g := range sdk.Gaps {
			r.Gaps = append(r.Gaps, Gap{Where: Go, Op: g.Op, Field: g.Field, Why: g.Cause})
		}
		out[filepath.Join("go", name, name+".go")] = sdk.Source
	}

	// Everything left is derived from the document's bytes, which is the same
	// input EmitSpec takes. One pipeline, reached two ways.
	rest, err := render(doc, name, want)
	if err != nil {
		return nil, err
	}
	for rel, body := range rest.files {
		out[rel] = body
	}
	r.Ops, r.Types = rest.ops, rest.types
	r.Gaps = append(r.Gaps, rest.gaps...)
	sortGaps(r.Gaps)
	return r, out.flush(r, o.Dir)
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
	for _, p := range o.Only {
		if p == Go {
			return nil, fmt.Errorf("opgen: the Go SDK speaks the ZAP wire, which a document does not describe — generate it from the app with Emit")
		}
	}
	s, err := Read(doc)
	if err != nil {
		return nil, err
	}
	name, err := called(o.Name, s.Name)
	if err != nil {
		return nil, err
	}
	out, err := render(doc, name, chosen(o.Only))
	if err != nil {
		return nil, err
	}
	r := &Result{Dir: o.Dir, Ops: out.ops, Types: out.types, Gaps: out.gaps}
	return r, out.files.flush(r, o.Dir)
}

// drawn is what a document alone yields.
type drawn struct {
	files      sheet
	gaps       []Gap
	ops, types int
}

// render is the one derivation from a document, used by both doors.
func render(doc []byte, name string, want map[zip.Projection]bool) (*drawn, error) {
	s, err := Read(doc)
	if err != nil {
		return nil, err
	}
	out := &drawn{files: sheet{}, ops: len(s.Ops), types: len(s.Types)}
	// A gap is reported for a projection this run is making. Naming one for a
	// leg nobody asked for describes a client that was not generated.
	for _, op := range s.Ops {
		if len(op.Unbound) == 0 {
			continue
		}
		why := "the address carries " + strings.Join(op.Unbound, ", ") + ", which the input type has no field for"
		for _, p := range []zip.Projection{Cpp, Rust} {
			if want[p] {
				out.gaps = append(out.gaps, Gap{Where: p, Op: op.ID, Field: strings.Join(op.Unbound, ","), Why: why})
			}
		}
	}
	sortGaps(out.gaps)

	if want[Commands] {
		cmds, err := zip.CommandsFromSpec(doc)
		if err != nil {
			return nil, fmt.Errorf("opgen: the command tree: %w", err)
		}
		b, err := json.MarshalIndent(cmds, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("opgen: rendering the command tree: %w", err)
		}
		out.files["commands.json"] = append(b, '\n')
	}
	if want[Rust] {
		for path, body := range rust(s, name) {
			out.files[filepath.Join("rust", path)] = body
		}
	}
	if want[Cpp] {
		for path, body := range cpp(s, name) {
			out.files[filepath.Join("cpp", path)] = body
		}
	}
	return out, nil
}

// called settles what the generated package, crate and namespace are named.
//
// The name reaches three languages, so it has to be an identifier in all of
// them: lower case, starting with a letter. A service whose own name is not one
// says so here rather than emitting a crate that will not build.
func called(given, fallback string) (string, error) {
	name := given
	if name == "" {
		name = snake(fallback)
	}
	if !identifier(name) {
		if given == "" {
			return "", fmt.Errorf("opgen: %q is not a name a package, a crate and a namespace can share; pass one that is", fallback)
		}
		return "", fmt.Errorf("opgen: %q is not a name a package, a crate and a namespace can share", given)
	}
	return name, nil
}

// identifier is the intersection of what Go, Rust and C++ accept.
func identifier(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
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

// sheet is everything a run will write, held until all of it is rendered.
//
// A run that put its files down as it made them would leave a document and a
// tool list behind when the SDK after them would not render — a half-described
// service, on disk, that reads as a whole one. Nothing lands until everything
// is in hand.
type sheet map[string][]byte

// flush writes the sheet, each file atomically, and records what it wrote.
func (s sheet) flush(r *Result, dir string) error {
	names := make([]string, 0, len(s))
	for rel := range s {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		dest := filepath.Join(dir, rel)
		if d := filepath.Dir(dest); d != "" {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return fmt.Errorf("opgen: %s: %w", rel, err)
			}
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, s[rel], 0o644); err != nil {
			return fmt.Errorf("opgen: %s: %w", rel, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("opgen: %s: %w", rel, err)
		}
		r.Files = append(r.Files, rel)
	}
	return nil
}

func sortGaps(g []Gap) {
	sort.Slice(g, func(i, j int) bool {
		if g[i].Where != g[j].Where {
			return g[i].Where < g[j].Where
		}
		if g[i].Op != g[j].Op {
			return g[i].Op < g[j].Op
		}
		return g[i].Field < g[j].Field
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
