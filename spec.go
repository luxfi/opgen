package opgen

// Reading the document back.
//
// The OpenAPI document is the intermediate form every client leg is generated
// from, so this is where it is turned back into the shape it came from: one
// request type and one reply type per operation.
//
// The document spells an operation's input two ways. A body op carries it as a
// schema under requestBody, named in components. A bodyless op has no body, so
// its input is projected onto the parameter list and its type name never
// appears anywhere. Both spellings come from ONE Go struct, and a client that
// took the second one literally would offer a method with eleven positional
// arguments where the service has a single value. So a bodyless op's input is
// reassembled here, under the operation's own name, and every generated method
// then has the same shape: one value in, one value out.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Surface is a service's API as the document describes it: the operations a
// client can call and the types those operations move.
type Surface struct {
	Name    string
	Version string
	Summary string
	Ops     []Op
	Types   []Type
}

// Op is one operation: an address, a name, and the two types either side of it.
type Op struct {
	ID      string // the operationId — the one identity across every projection
	Method  string // GET, POST, ...
	Route   string // the URL template, "{name}" spelling
	Summary string
	Detail  string

	In  string // the request type's name, "" when the operation takes nothing
	Out string // the reply type's name, "" when it answers nothing

	// Segments are the In fields that fill the route's templated segments, in
	// segment order. Query are the In fields that ride the query string. What
	// is in neither list is the JSON body, and Body says whether there is one.
	Segments []string
	Query    []string
	Body     bool
}

// Type is one named structure the wire carries.
type Type struct {
	Name      string
	Detail    string
	Fields    []Field
	Synthetic bool // reassembled from a parameter list rather than read from components
}

// Field is one member of a type.
type Field struct {
	Name   string // the JSON name, which is the name on the wire
	Detail string
	Kind   Kind
	// Cyclic marks a field whose type reaches back to the type that holds it.
	// A language that lays a struct out by value cannot do that, so the
	// emitters put such a field behind a pointer — and only such a field.
	Cyclic bool
}

// Sort is what a value is, reduced to the small set every target language can
// spell. It is deliberately not the JSON Schema keyword set: two keywords that
// a client must render differently (a plain string and a base64 one) are two
// sorts here, and two that it renders the same are one.
type Sort string

const (
	Text   Sort = "text"   // a string
	Whole  Sort = "whole"  // an integer, of Bits width
	Real   Sort = "real"   // a floating point number, of Bits width
	Flag   Sort = "flag"   // a boolean
	Moment Sort = "moment" // an RFC 3339 timestamp, carried as text
	Octets Sort = "octets" // bytes, carried as base64 text
	List   Sort = "list"   // an array of Elem
	Table  Sort = "table"  // an object keyed by string, valued Elem
	Free   Sort = "free"   // any JSON value
	Named  Sort = "named"  // a reference to a declared type
)

// Kind is one value's type.
type Kind struct {
	Sort   Sort
	Bits   int    // 8, 16, 32 or 64, for Whole and Real
	Signed bool   // for Whole
	Elem   *Kind  // the element of a List, the value of a Table
	Ref    string // the type a Named refers to
}

// ---- reading the document ---------------------------------------------------

// Read turns an OpenAPI document into a Surface.
//
// It refuses a document it cannot generate a client from rather than skipping
// the operation: an SDK missing a method its service answers is a drift nobody
// notices until a caller needs the method.
func Read(doc []byte) (*Surface, error) {
	var d document
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("opgen: reading the document: %w", err)
	}
	s := &Surface{Name: d.Info.Title, Version: d.Info.Version, Summary: d.Info.Description}
	if s.Name == "" {
		return nil, fmt.Errorf("opgen: the document names no service (info.title is empty)")
	}

	declared := map[string]Type{}
	for name, sch := range d.Components.Schemas {
		declared[name] = Type{Name: name, Detail: sch.Description, Fields: fields(sch)}
	}

	for route, item := range d.Paths {
		for method, op := range item {
			method = strings.ToUpper(method)
			if !verb(method) {
				continue // "parameters" and other path-level keys are not operations
			}
			read, err := operation(method, route, op, declared)
			if err != nil {
				return nil, err
			}
			s.Ops = append(s.Ops, read)
		}
	}
	sort.Slice(s.Ops, func(i, j int) bool { return s.Ops[i].ID < s.Ops[j].ID })

	// Only types an operation can reach are emitted. A component nothing calls
	// is a type no client needs, and emitting it would put a name in the SDK
	// that the service does not answer on.
	keep := map[string]bool{}
	for _, o := range s.Ops {
		reach(declared, o.In, keep)
		reach(declared, o.Out, keep)
	}
	for name := range keep {
		s.Types = append(s.Types, declared[name])
	}
	sort.Slice(s.Types, func(i, j int) bool { return s.Types[i].Name < s.Types[j].Name })
	mark(s.Types)
	return s, nil
}

// operation reads one operation, reassembling a bodyless op's input.
func operation(method, route string, op specOp, declared map[string]Type) (Op, error) {
	if op.OperationID == "" {
		return Op{}, fmt.Errorf("opgen: %s %s has no operationId, so no method can be named for it", method, route)
	}
	o := Op{
		ID: op.OperationID, Method: method, Route: route,
		Summary: op.Summary, Detail: op.Description,
	}
	// An op can declare its own statuses, so the answer is not always under
	// "200". Take the lowest success that carries a body: they all describe the
	// same type — the document repeats one schema under every declared code.
	for _, code := range success(op.Responses) {
		if m, ok := op.Responses[code].Content["application/json"]; ok {
			o.Out = refName(m.Schema.Ref)
			break
		}
	}

	segments := templated(route)
	if body := op.body(); body != nil {
		o.In, o.Body = refName(body.Schema.Ref), true
		o.Segments = segments
		// A path parameter is a field of the body type that the URL carries
		// instead. It stays in the type — the service reads one struct — and
		// the emitters know not to write it into the JSON twice.
		return o, nil
	}
	if len(op.Parameters) == 0 {
		return o, nil // takes nothing
	}

	// Bodyless: reassemble the input the parameters were projected from.
	name := title(op.OperationID)
	t := Type{Name: name, Synthetic: true, Detail: op.Summary}
	for _, p := range op.Parameters {
		t.Fields = append(t.Fields, Field{Name: p.Name, Detail: p.Description, Kind: kind(p.Schema)})
		switch p.In {
		case "path":
			o.Segments = append(o.Segments, p.Name)
		case "query":
			o.Query = append(o.Query, p.Name)
		}
	}
	if _, taken := declared[name]; taken {
		return Op{}, fmt.Errorf("opgen: %s needs a request type named %q and the document already declares one", op.OperationID, name)
	}
	declared[name] = t
	o.In = name
	return o, nil
}

// success is the 2xx codes a set of responses declares, lowest first.
func success(responses map[string]struct {
	Content map[string]specMedia `json:"content"`
}) []string {
	var out []string
	for code := range responses {
		if n, err := strconv.Atoi(code); err == nil && n >= 200 && n < 300 {
			out = append(out, code)
		}
	}
	sort.Strings(out)
	return out
}

// fields reads one schema's properties, in the one order a map does not have.
func fields(s specSchema) []Field {
	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Field, 0, len(names))
	for _, n := range names {
		p := s.Properties[n]
		out = append(out, Field{Name: n, Detail: p.Description, Kind: kind(p)})
	}
	return out
}

// kind reduces one JSON Schema to what a client has to spell.
func kind(s specSchema) Kind {
	if s.Ref != "" {
		return Kind{Sort: Named, Ref: refName(s.Ref)}
	}
	switch s.Type {
	case "string":
		switch {
		case s.Format == "date-time":
			return Kind{Sort: Moment}
		case s.ContentEncoding == "base64":
			return Kind{Sort: Octets}
		}
		return Kind{Sort: Text}
	case "integer":
		// The document spells every Go integer width, and six of the eight
		// formats are not standard OpenAPI — they are the Go spelling. Reading
		// only int32 and int64 would silently widen every uint32 to a signed
		// 64-bit and let a client offer a range the service will refuse.
		if w, ok := widths[s.Format]; ok {
			return Kind{Sort: Whole, Bits: w.bits, Signed: w.signed}
		}
		return Kind{Sort: Whole, Bits: 64, Signed: true}
	case "number":
		if s.Format == "float" {
			return Kind{Sort: Real, Bits: 32}
		}
		return Kind{Sort: Real, Bits: 64}
	case "boolean":
		return Kind{Sort: Flag}
	case "array":
		e := Kind{Sort: Free}
		if s.Items != nil {
			e = kind(*s.Items)
		}
		return Kind{Sort: List, Elem: &e}
	case "object":
		if s.AdditionalProperties != nil {
			e := kind(*s.AdditionalProperties)
			return Kind{Sort: Table, Elem: &e}
		}
		// An object with neither properties nor a value type is the document's
		// way of writing "any JSON", which is what a Go `any` field becomes.
		return Kind{Sort: Free}
	}
	return Kind{Sort: Free}
}

// widths is every integer format the document can carry.
var widths = map[string]struct {
	bits   int
	signed bool
}{
	"int8": {8, true}, "int16": {16, true}, "int32": {32, true}, "int64": {64, true},
	"uint8": {8, false}, "uint16": {16, false}, "uint32": {32, false}, "uint64": {64, false},
}

// reach marks name and everything it refers to.
func reach(declared map[string]Type, name string, keep map[string]bool) {
	if name == "" || keep[name] {
		return
	}
	t, ok := declared[name]
	if !ok {
		return
	}
	keep[name] = true
	for _, f := range t.Fields {
		for _, r := range refs(f.Kind) {
			reach(declared, r, keep)
		}
	}
}

// refs is every declared type one kind reaches.
func refs(k Kind) []string {
	switch k.Sort {
	case Named:
		return []string{k.Ref}
	case List, Table:
		if k.Elem != nil {
			return refs(*k.Elem)
		}
	}
	return nil
}

// mark finds the fields that make a type reach itself.
//
// The document cannot say whether a Go field was a pointer: `*Party` and
// `Party` are both a $ref. So a self-referential type reads as one that
// contains itself by value, which no language with a fixed layout can lay out.
// The fields on a cycle are marked here and the emitters put those, and only
// those, behind a pointer.
func mark(types []Type) {
	at := map[string]int{}
	for i, t := range types {
		at[t.Name] = i
	}
	const (
		fresh = iota
		open
		done
	)
	state := make([]int, len(types))
	var walk func(i int)
	walk = func(i int) {
		state[i] = open
		for fi := range types[i].Fields {
			f := &types[i].Fields[fi]
			// Only a direct reference lays a type out inside another. A list or
			// a table holds its elements elsewhere, so a cycle through one is
			// already indirect and needs no pointer.
			if f.Kind.Sort != Named {
				continue
			}
			j, ok := at[f.Kind.Ref]
			if !ok {
				continue
			}
			switch state[j] {
			case open:
				f.Cyclic = true // a back edge: this is what closes the cycle
			case fresh:
				walk(j)
			}
		}
		state[i] = done
	}
	for i := range types {
		if state[i] == fresh {
			walk(i)
		}
	}
}

// templated is the route's templated segments, in the order the URL has them.
func templated(route string) []string {
	var out []string
	for _, p := range strings.Split(route, "/") {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			out = append(out, p[1:len(p)-1])
		}
	}
	return out
}

func refName(ref string) string {
	const prefix = "#/components/schemas/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ""
}

func verb(s string) bool {
	switch s {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

// ---- the document's own shape -----------------------------------------------

type document struct {
	Info struct {
		Title       string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
	} `json:"info"`
	Paths      map[string]map[string]specOp `json:"paths"`
	Components struct {
		Schemas map[string]specSchema `json:"schemas"`
	} `json:"components"`
}

type specOp struct {
	OperationID string      `json:"operationId"`
	Summary     string      `json:"summary"`
	Description string      `json:"description"`
	Parameters  []specParam `json:"parameters"`
	RequestBody *struct {
		Content map[string]specMedia `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]specMedia `json:"content"`
	} `json:"responses"`
}

func (o specOp) body() *specMedia {
	if o.RequestBody == nil {
		return nil
	}
	if m, ok := o.RequestBody.Content["application/json"]; ok {
		return &m
	}
	return nil
}

type specMedia struct {
	Schema specSchema `json:"schema"`
}

type specParam struct {
	Name        string     `json:"name"`
	In          string     `json:"in"`
	Description string     `json:"description"`
	Required    bool       `json:"required"`
	Schema      specSchema `json:"schema"`
}

type specSchema struct {
	Ref                  string                `json:"$ref"`
	Type                 string                `json:"type"`
	Description          string                `json:"description"`
	Format               string                `json:"format"`
	ContentEncoding      string                `json:"contentEncoding"`
	Properties           map[string]specSchema `json:"properties"`
	Items                *specSchema           `json:"items"`
	AdditionalProperties *specSchema           `json:"additionalProperties"`
}
