package opgen

// The C++ client.
//
// One header. The types the service moves, one method per operation, and a
// Transport the caller supplies — the same shape as the Rust crate, for the
// same reason: the contract is ours, the bytes are the program's.
//
// It uses nlohmann/json, which is the one dependency. C++ has no JSON in its
// standard library, so the choice is a header everyone already has or three
// hundred lines of hand-written parser inside every generated SDK — and a
// parser we wrote is a parser we would have to be right about forever.

import (
	"fmt"
	"strings"
)

var cppKeywords = map[string]bool{
	"alignas": true, "alignof": true, "and": true, "asm": true, "auto": true, "bitand": true,
	"bitor": true, "bool": true, "break": true, "case": true, "catch": true, "char": true,
	"class": true, "compl": true, "concept": true, "const": true, "consteval": true,
	"constexpr": true, "constinit": true, "continue": true, "co_await": true,
	"co_return": true, "co_yield": true, "decltype": true, "default": true, "delete": true,
	"do": true, "double": true, "dynamic_cast": true, "else": true, "enum": true,
	"explicit": true, "export": true, "extern": true, "false": true, "float": true,
	"for": true, "friend": true, "goto": true, "if": true, "inline": true, "int": true,
	"long": true, "mutable": true, "namespace": true, "new": true, "noexcept": true,
	"not": true, "nullptr": true, "operator": true, "or": true, "private": true,
	"protected": true, "public": true, "register": true, "requires": true, "return": true,
	"short": true, "signed": true, "sizeof": true, "static": true, "static_cast": true,
	"struct": true, "switch": true, "template": true, "this": true, "throw": true,
	"true": true, "try": true, "typedef": true, "typeid": true, "typename": true,
	"union": true, "unsigned": true, "using": true, "virtual": true, "void": true,
	"volatile": true, "wchar_t": true, "while": true, "xor": true,
}

// cpp renders the library: path relative to the cpp/ directory, and content.
func cpp(s *Surface, name string) map[string][]byte {
	ns := safe(snake(name), cppKeywords)
	header := ns + ".hpp"

	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("// Code generated from %s's typed ops by opgen. DO NOT EDIT.\n//\n", s.Name)
	p("// %s\n//\n", oneLine(s.Summary, s.Name+"'s API."))
	p("// A field absent from an answer keeps its default. The service is written\n")
	p("// in Go, where a struct field is always present and an absent one arrives as\n")
	p("// its zero value, so refusing an answer for a missing key would refuse\n// answers the service considers complete.\n")
	p("#pragma once\n\n")
	p("#include <cstdint>\n#include <map>\n#include <memory>\n#include <sstream>\n#include <stdexcept>\n#include <string>\n#include <vector>\n\n")
	p("#include <nlohmann/json.hpp>\n\n")
	p("namespace %s {\n\n", ns)
	p("%s\n", cppRuntime)

	types := order(s.Types)

	// Declared first, because a type on a cycle is reached through a pointer
	// and a pointer to an incomplete type is enough.
	p("// The types this service moves.\n")
	for _, t := range types {
		p("struct %s;\n", title(t.Name))
	}
	p("\n")

	for _, t := range types {
		if d := oneLine(t.Detail, ""); d != "" {
			p("/// %s\n", d)
		}
		p("struct %s {\n", title(t.Name))
		for _, f := range t.Fields {
			if d := oneLine(f.Detail, ""); d != "" {
				p("    /// %s\n", d)
			}
			p("    %s %s{};\n", cppType(f.Kind, f.Cyclic), cppField(f.Name))
		}
		p("};\n\n")
	}

	// Declarations before definitions, so a type that contains another can be
	// written before the other's conversions are defined.
	for _, t := range types {
		p("inline void to_json(nlohmann::json& j, const %s& v);\n", title(t.Name))
		p("inline void from_json(const nlohmann::json& j, %s& v);\n", title(t.Name))
	}
	p("\n")

	for _, t := range types {
		n := title(t.Name)
		p("inline void to_json(nlohmann::json& j, const %s& v) {\n", n)
		p("    j = nlohmann::json::object();\n")
		if len(t.Fields) == 0 {
			p("    (void)v;\n")
		}
		for _, f := range t.Fields {
			field := cppField(f.Name)
			if f.Cyclic {
				p("    if (v.%s) { j[%q] = *v.%s; } else { j[%q] = nullptr; }\n", field, f.Name, field, f.Name)
				continue
			}
			p("    j[%q] = v.%s;\n", f.Name, field)
		}
		p("}\n\n")

		p("inline void from_json(const nlohmann::json& j, %s& v) {\n", n)
		if len(t.Fields) == 0 {
			p("    (void)j;\n    (void)v;\n")
		}
		for _, f := range t.Fields {
			field := cppField(f.Name)
			p("    if (auto it = j.find(%q); it != j.end() && !it->is_null()) {\n", f.Name)
			if f.Cyclic {
				p("        v.%s = std::make_shared<%s>(it->get<%s>());\n", field, title(f.Kind.Ref), title(f.Kind.Ref))
			} else {
				p("        it->get_to(v.%s);\n", field)
			}
			p("    }\n")
		}
		p("}\n\n")
	}

	p("/// %s's operations.\n", s.Name)
	p("class Client {\n public:\n")
	p("    explicit Client(Transport& transport) : transport_(transport) {}\n")
	for _, o := range s.Ops {
		if len(o.Unbound) > 0 {
			continue // no method, rather than one that names a field the type has not got
		}
		p("\n%s", cppMethod(s, o))
	}
	p("\n private:\n    Transport& transport_;\n};\n\n")
	p("}  // namespace %s\n", ns)

	build := fmt.Sprintf(`# Code generated from %s's typed ops by opgen. DO NOT EDIT.
cmake_minimum_required(VERSION 3.16)
project(%s LANGUAGES CXX)

find_package(nlohmann_json 3 REQUIRED)

add_library(%s INTERFACE)
target_include_directories(%s INTERFACE ${CMAKE_CURRENT_SOURCE_DIR}/include)
target_compile_features(%s INTERFACE cxx_std_20)
target_link_libraries(%s INTERFACE nlohmann_json::nlohmann_json)
`, s.Name, ns, ns, ns, ns, ns)

	return map[string][]byte{
		"CMakeLists.txt":               []byte(build),
		"include/" + ns + "/" + header: []byte(b.String()),
	}
}

// cppMethod renders one operation.
func cppMethod(s *Surface, o Op) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	if d := oneLine(o.Summary, ""); d != "" {
		p("    /// %s\n", d)
	}
	if o.Detail != "" && oneLine(o.Detail, "") != oneLine(o.Summary, "") {
		p("    ///\n    /// %s\n", oneLine(o.Detail, ""))
	}
	p("    ///\n    /// %s %s\n", o.Method, o.Route)

	out := "void"
	if o.Out != "" {
		out = title(o.Out)
	}
	method := safe(snake(o.ID), cppKeywords)
	if o.In == "" {
		p("    %s %s() {\n", out, method)
	} else {
		p("    %s %s(const %s& input) {\n", out, method, title(o.In))
	}

	if len(o.Segments) == 0 {
		p("        std::string target = %q;\n", o.Route)
	} else {
		in := typeOf(s, o.In)
		p("        std::string target;\n")
		for _, part := range routeParts(o.Route) {
			if part.param == "" {
				p("        target += %q;\n", part.text)
				continue
			}
			f := fieldOf(in, part.param)
			p("        target += encode(%s);\n", cppText(f, "input."+cppField(part.param)))
		}
	}
	if len(o.Query) > 0 {
		in := typeOf(s, o.In)
		first := true
		for _, q := range o.Query {
			f := fieldOf(in, q)
			if !urlSpelled(f.Kind) {
				continue
			}
			sep := "&"
			if first {
				sep, first = "?", false
			}
			p("        target += %q;\n", sep+q+"=")
			p("        target += encode(%s);\n", cppText(f, "input."+cppField(q)))
		}
	}

	if o.Body {
		p("        nlohmann::json body = input;\n")
		p("        const std::string encoded = body.dump();\n")
		p("        const Reply reply = transport_.send(%q, target, &encoded);\n", o.Method)
	} else {
		p("        const Reply reply = transport_.send(%q, target, nullptr);\n", o.Method)
	}
	p("        if (reply.status < 200 || reply.status >= 300) {\n")
	p("            throw Refused(reply.status, reply.body);\n        }\n")
	if o.Out == "" {
		p("    }\n")
		return b.String()
	}
	p("        return nlohmann::json::parse(reply.body).get<%s>();\n", out)
	p("    }\n")
	return b.String()
}

// cppText spells one value as the text a URL carries.
func cppText(f Field, ident string) string {
	switch f.Kind.Sort {
	case Text, Moment, Octets:
		return ident
	case Flag:
		return fmt.Sprintf("std::string(%s ? \"true\" : \"false\")", ident)
	case List:
		return fmt.Sprintf("join(%s)", ident)
	default:
		return fmt.Sprintf("number(%s)", ident)
	}
}

// cppType spells one kind.
func cppType(k Kind, cyclic bool) string {
	switch k.Sort {
	case Text, Moment, Octets:
		return "std::string"
	case Whole:
		if k.Signed {
			return fmt.Sprintf("std::int%d_t", k.Bits)
		}
		return fmt.Sprintf("std::uint%d_t", k.Bits)
	case Real:
		if k.Bits == 32 {
			return "float"
		}
		return "double"
	case Flag:
		return "bool"
	case List:
		return "std::vector<" + cppType(*k.Elem, false) + ">"
	case Table:
		return "std::map<std::string, " + cppType(*k.Elem, false) + ">"
	case Named:
		if cyclic {
			// A type that reaches itself has no size until something on the
			// cycle is a pointer.
			return "std::shared_ptr<" + title(k.Ref) + ">"
		}
		return title(k.Ref)
	}
	return "nlohmann::json"
}

func cppField(name string) string { return safe(snake(name), cppKeywords) }

// cppRuntime is the part of every generated header that is the same in every
// generated header.
const cppRuntime = `/// One answer, as bytes.
struct Reply {
    int status = 0;
    std::string body;
};

/// How the bytes travel. The SDK knows the contract; the program it lands in
/// already has an HTTP client, and this is where it plugs in.
///
/// ` + "`target`" + ` is an absolute path with its query string, so the base
/// address belongs to the implementation and not to the call.
struct Transport {
    virtual ~Transport() = default;
    virtual Reply send(const std::string& method, const std::string& target,
                       const std::string* body) = 0;
};

/// The service answered, and refused.
class Refused : public std::runtime_error {
 public:
    Refused(int status, std::string body)
        : std::runtime_error("status " + std::to_string(status) + ": " + body),
          status_(status), body_(std::move(body)) {}

    int status() const noexcept { return status_; }
    const std::string& body() const noexcept { return body_; }

 private:
    int status_;
    std::string body_;
};

/// Percent-encodes one path segment or query value. Everything outside the
/// unreserved set is escaped, which is correct in both places.
inline std::string encode(const std::string& s) {
    static const char* hex = "0123456789ABCDEF";
    std::string out;
    out.reserve(s.size());
    for (unsigned char c : s) {
        if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
            (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~') {
            out.push_back(static_cast<char>(c));
        } else {
            out.push_back('%');
            out.push_back(hex[c >> 4]);
            out.push_back(hex[c & 0x0F]);
        }
    }
    return out;
}

/// Spells one number the way the URL carries it.
template <typename T>
inline std::string number(const T& v) {
    std::ostringstream out;
    out << v;
    return out.str();
}

/// Spells a repeated value the way the service reads one: comma separated.
template <typename T>
inline std::string join(const std::vector<T>& vs) {
    std::string out;
    for (std::size_t i = 0; i < vs.size(); ++i) {
        if (i > 0) out.push_back(',');
        out += number(vs[i]);
    }
    return out;
}

/// A string joins as itself, not through a stream.
template <>
inline std::string join<std::string>(const std::vector<std::string>& vs) {
    std::string out;
    for (std::size_t i = 0; i < vs.size(); ++i) {
        if (i > 0) out.push_back(',');
        out += vs[i];
    }
    return out;
}
`
