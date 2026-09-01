package opgen

// The Rust client.
//
// One crate: the types the service moves, one method per operation, and a
// Transport the caller supplies. It depends on serde and serde_json and on
// nothing else — no HTTP stack, no async runtime, no TLS.
//
// That is deliberate. What a generated client knows that a hand-written one
// does not is the CONTRACT: the addresses, the types either side of them, and
// how a value is spelled on the wire. It does not know which HTTP library the
// program it lands in already links, and choosing one for that program is how a
// generated SDK becomes the reason a build has two TLS stacks in it. So the
// bytes are the caller's and the types are ours.

import (
	"fmt"
	"sort"
	"strings"
)

var rustKeywords = map[string]bool{
	"as": true, "async": true, "await": true, "box": true, "break": true, "const": true,
	"continue": true, "crate": true, "dyn": true, "else": true, "enum": true, "extern": true,
	"false": true, "fn": true, "for": true, "if": true, "impl": true, "in": true, "let": true,
	"loop": true, "match": true, "mod": true, "move": true, "mut": true, "pub": true,
	"ref": true, "return": true, "self": true, "static": true, "struct": true, "super": true,
	"trait": true, "true": true, "type": true, "unsafe": true, "use": true, "where": true,
	"while": true, "abstract": true, "become": true, "do": true, "final": true, "macro": true,
	"override": true, "priv": true, "try": true, "typeof": true, "unsized": true,
	"virtual": true, "yield": true,
}

// rust renders the crate: path relative to the rust/ directory, and content.
func rust(s *Surface, name string) map[string][]byte {
	version := semver(s.Version)
	manifest := fmt.Sprintf(`# Code generated from %s's typed ops by opgen. DO NOT EDIT.
[package]
name = "%s"
version = "%s"
edition = "2021"

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"
`, s.Name, name, version)

	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("// Code generated from %s's typed ops by opgen. DO NOT EDIT.\n//\n", s.Name)
	p("// %s\n", oneLine(s.Summary, s.Name+"'s API."))
	p("//\n// Every field is `#[serde(default)]`: the service is written in Go, where a\n")
	p("// struct field is always present and an absent one arrives as its zero value.\n")
	p("// A client that made absence an error would refuse answers the service\n// considers complete.\n\n")
	p("#![allow(clippy::all)]\n\n")
	p("use serde::{Deserialize, Serialize};\n\n")
	p("%s\n", rustRuntime)
	if addresses(s) {
		p("%s\n", rustEncode)
	}

	for _, t := range s.Types {
		if d := oneLine(t.Detail, ""); d != "" {
			p("/// %s\n", d)
		}
		p("#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]\n")
		p("pub struct %s {\n", title(t.Name))
		for _, f := range t.Fields {
			if d := oneLine(f.Detail, ""); d != "" {
				p("    /// %s\n", d)
			}
			ident := safe(snake(f.Name), rustKeywords)
			if ident != f.Name {
				p("    #[serde(rename = %q, default)]\n", f.Name)
			} else {
				p("    #[serde(default)]\n")
			}
			p("    pub %s: %s,\n", ident, rustType(f.Kind, f.Cyclic))
		}
		p("}\n\n")
	}

	p("/// %s's operations.\n", s.Name)
	p("pub struct Client<T: Transport> {\n    transport: T,\n}\n\n")
	p("impl<T: Transport> Client<T> {\n")
	p("    /// Returns a client that calls over `transport`.\n")
	p("    pub fn new(transport: T) -> Self {\n        Client { transport }\n    }\n")
	for _, o := range s.Ops {
		if len(o.Unbound) > 0 {
			continue // no method, rather than one that names a field the type has not got
		}
		p("\n%s", rustMethod(s, o))
	}
	p("}\n")

	return map[string][]byte{
		name + "/Cargo.toml": []byte(manifest),
		name + "/src/lib.rs": []byte(b.String()),
	}
}

// semver is the document's version in the one spelling cargo accepts.
//
// A Go service spells its version with a leading v — node's platformvm declares
// version.Current.String(), which is "v1.36.181" — and cargo refuses to parse a
// manifest carrying one. Anything that is still not three numbers becomes
// 0.0.0: a crate that will not parse is worse than one whose version says
// nothing.
func semver(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return "0.0.0"
	}
	for i, p := range parts {
		if i == 2 {
			// The patch may carry a pre-release or build suffix, which cargo
			// takes; only the number in front of it has to be a number.
			p = strings.SplitN(strings.SplitN(p, "-", 2)[0], "+", 2)[0]
		}
		if p == "" {
			return "0.0.0"
		}
		for j := 0; j < len(p); j++ {
			if p[j] < '0' || p[j] > '9' {
				return "0.0.0"
			}
		}
	}
	return v
}

// rustMethod renders one operation.
func rustMethod(s *Surface, o Op) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	if d := oneLine(o.Summary, ""); d != "" {
		p("    /// %s\n", d)
	}
	if o.Detail != "" && oneLine(o.Detail, "") != oneLine(o.Summary, "") {
		p("    ///\n    /// %s\n", oneLine(o.Detail, ""))
	}
	p("    ///\n    /// `%s %s`\n", o.Method, o.Route)

	out := "()"
	if o.Out != "" {
		out = title(o.Out)
	}
	if o.In == "" {
		p("    pub fn %s(&self) -> Result<%s, Error> {\n", safe(snake(o.ID), rustKeywords), out)
	} else {
		p("    pub fn %s(&self, input: &%s) -> Result<%s, Error> {\n", safe(snake(o.ID), rustKeywords), title(o.In), out)
	}

	// The address. A map or a free value has no spelling in a URL, so the
	// service does not read one from there and neither is one written.
	in := typeOf(s, o.In)
	var carried []string
	for _, q := range o.Query {
		if urlSpelled(fieldOf(in, q).Kind) {
			carried = append(carried, q)
		}
	}
	moves := len(o.Segments) > 0 || len(carried) > 0
	switch {
	case !moves:
		p("        let target = String::from(%q);\n", o.Route)
	case len(o.Segments) == 0:
		p("        let mut target = String::from(%q);\n", o.Route)
	default:
		p("        let mut target = String::new();\n")
		for _, part := range routeParts(o.Route) {
			if part.param == "" {
				p("        target.push_str(%q);\n", part.text)
				continue
			}
			f := fieldOf(in, part.param)
			p("        target.push_str(&encode(&%s));\n", rustText(f, "input."+safe(snake(part.param), rustKeywords)))
		}
	}
	for i, q := range carried {
		lead := "&"
		if i == 0 {
			lead = "?"
		}
		f := fieldOf(in, q)
		ident := "input." + safe(snake(q), rustKeywords)
		p("        target.push_str(%q);\n", lead+q+"=")
		p("        target.push_str(&encode(&%s));\n", rustText(f, ident))
	}

	// The body.
	switch {
	case o.Body:
		p("        let body = serde_json::to_vec(input).map_err(|e| Error::Encoding(e.to_string()))?;\n")
		p("        let reply = self.transport.send(%q, &target, Some(&body))?;\n", o.Method)
	default:
		p("        let reply = self.transport.send(%q, &target, None)?;\n", o.Method)
	}
	p("        if reply.status < 200 || reply.status >= 300 {\n")
	p("            return Err(Error::Status {\n                status: reply.status,\n                body: String::from_utf8_lossy(&reply.body).into_owned(),\n            });\n        }\n")
	if o.Out == "" {
		p("        Ok(())\n")
	} else {
		p("        serde_json::from_slice(&reply.body).map_err(|e| Error::Encoding(e.to_string()))\n")
	}
	p("    }\n")
	return b.String()
}

// rustText spells one value as the text a URL carries.
func rustText(f Field, ident string) string {
	switch f.Kind.Sort {
	case Text, Moment, Octets:
		return ident
	case List:
		return fmt.Sprintf("%s.iter().map(|v| v.to_string()).collect::<Vec<_>>().join(\",\")", ident)
	default:
		return ident + ".to_string()"
	}
}

// rustType spells one kind.
func rustType(k Kind, cyclic bool) string {
	switch k.Sort {
	case Text, Moment, Octets:
		return "String"
	case Whole:
		if k.Signed {
			return fmt.Sprintf("i%d", k.Bits)
		}
		return fmt.Sprintf("u%d", k.Bits)
	case Real:
		return fmt.Sprintf("f%d", k.Bits)
	case Flag:
		return "bool"
	case List:
		return "Vec<" + rustType(*k.Elem, false) + ">"
	case Table:
		return "std::collections::BTreeMap<String, " + rustType(*k.Elem, false) + ">"
	case Named:
		if cyclic {
			// A type that reaches itself has no size until something on the
			// cycle is a pointer. Option, because the document cannot say
			// whether the Go field was one, and a self-reference has to be
			// allowed to end.
			return "Option<Box<" + title(k.Ref) + ">>"
		}
		return title(k.Ref)
	}
	return "serde_json::Value"
}

// rustRuntime is the part of every generated crate that is the same in every
// generated crate. It is written out rather than shipped as a crate of its own
// so the SDK a user adds is one dependency and there is nothing to keep level
// with it.
const rustRuntime = `/// One answer, as bytes.
pub struct Reply {
    pub status: u16,
    pub body: Vec<u8>,
}

/// How the bytes travel. The SDK knows the contract; the program it lands in
/// already has an HTTP client, and this is where it plugs in.
///
/// ` + "`target`" + ` is an absolute path with its query string, so the base
/// address belongs to the implementation and not to the call.
pub trait Transport {
    fn send(&self, method: &str, target: &str, body: Option<&[u8]>) -> Result<Reply, Error>;
}

/// A borrowed transport is a transport, so one client can be handed a reference
/// to a transport the program already owns.
impl<T: Transport + ?Sized> Transport for &T {
    fn send(&self, method: &str, target: &str, body: Option<&[u8]>) -> Result<Reply, Error> {
        (**self).send(method, target, body)
    }
}

/// What can go wrong, told apart by whose fault it is.
#[derive(Clone, Debug, PartialEq)]
pub enum Error {
    /// The bytes did not get there.
    Transport(String),
    /// They got there and the service refused.
    Status { status: u16, body: String },
    /// A value would not go on or come off the wire.
    Encoding(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::Transport(m) => write!(f, "transport: {m}"),
            Error::Status { status, body } => write!(f, "status {status}: {body}"),
            Error::Encoding(m) => write!(f, "encoding: {m}"),
        }
    }
}

impl std::error::Error for Error {}
`

// rustEncode is emitted only for a service whose URLs carry a value. A crate
// that carried an encoder nothing calls would not compile under the dead-code
// lint, and silencing that lint would let real dead code in behind it.
const rustEncode = `/// Percent-encodes one path segment or query value. Everything outside the
/// unreserved set is escaped, which is correct in both places.
fn encode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.as_bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'.' | b'_' | b'~' => {
                out.push(*b as char)
            }
            _ => out.push_str(&format!("%{b:02X}")),
        }
    }
    out
}
`

// ---- shared shaping ---------------------------------------------------------

// part is one piece of a route: literal text, or the name of a segment a value
// fills.
type part struct {
	text  string
	param string
}

// routeParts splits "/v1/secrets/{name}/keys" into the literals and the values
// between them, so an emitter can build the address without a template engine.
func routeParts(route string) []part {
	var out []part
	rest := route
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			if rest != "" {
				out = append(out, part{text: rest})
			}
			return out
		}
		j := strings.Index(rest[i:], "}")
		if j < 0 {
			out = append(out, part{text: rest})
			return out
		}
		if i > 0 {
			out = append(out, part{text: rest[:i]})
		}
		out = append(out, part{param: rest[i+1 : i+j]})
		rest = rest[i+j+1:]
	}
}

// typeOf finds a declared type by name.
func typeOf(s *Surface, name string) Type {
	for _, t := range s.Types {
		if t.Name == name {
			return t
		}
	}
	return Type{}
}

// fieldOf finds one field. A route segment naming no field is carried as text,
// which is what the service does with it too.
func fieldOf(t Type, name string) Field {
	for _, f := range t.Fields {
		if f.Name == name {
			return f
		}
	}
	return Field{Name: name, Kind: Kind{Sort: Text}}
}

// urlSpelled says whether a value has a spelling in a URL at all. A map and a
// free value do not, which is why the service does not read them from one.
func urlSpelled(k Kind) bool {
	switch k.Sort {
	case Table, Free, Named:
		return false
	}
	return true
}

// addresses says whether any operation puts a value in its URL. A service whose
// every op takes a body and answers at a fixed address needs no encoder.
func addresses(s *Surface) bool {
	for _, o := range s.Ops {
		if len(o.Unbound) > 0 {
			continue // this op gets no method, so its address is never built
		}
		if len(o.Segments) > 0 {
			return true
		}
		in := typeOf(s, o.In)
		for _, q := range o.Query {
			if urlSpelled(fieldOf(in, q).Kind) {
				return true
			}
		}
	}
	return false
}

// oneLine reduces prose to a single line, so a doc comment cannot break out of
// the comment it is written into.
func oneLine(s, fallback string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if s == "" {
		return fallback
	}
	return s
}

// order sorts types so a definition comes after everything it contains by
// value. A field on a cycle is behind a pointer and does not constrain the
// order, which is what makes an order exist at all.
func order(types []Type) []Type {
	at := map[string]int{}
	for i, t := range types {
		at[t.Name] = i
	}
	seen := make([]bool, len(types))
	out := make([]Type, 0, len(types))
	var walk func(i int)
	walk = func(i int) {
		if seen[i] {
			return
		}
		seen[i] = true
		var deps []int
		for _, f := range types[i].Fields {
			if f.Cyclic {
				continue
			}
			for _, r := range refs(f.Kind) {
				if j, ok := at[r]; ok {
					deps = append(deps, j)
				}
			}
		}
		sort.Ints(deps)
		for _, j := range deps {
			walk(j)
		}
		out = append(out, types[i])
	}
	for i := range types {
		walk(i)
	}
	return out
}
