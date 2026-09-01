# opgen

One generator. A service's typed ops become its OpenAPI document, its MCP tool
list, its command tree and its Go, Rust and C++ SDKs — in one run, from one
source, with nothing written down twice.

## The source

The typed ops in Go. `zip.Post(app, "/v1/seal", handler, zip.WithOperationID(...))`
and nothing else. There is no IDL, no schema file, no per-language definition to
keep level. Adding an op adds a method to three SDKs, a tool to the agent
manifest, a command to the CLI and a path to the docs site.

## The two intermediate forms, and why there are two

    the OpenAPI document   the JSON edge. Complete for an HTTP client, and it
                           crosses a process boundary — so the Rust, C++ and
                           command-line legs can be generated for a service the
                           generator does not link.

    the op registry        the ZAP wire, where a field is an offset and a width.
                           The document cannot describe it: JSON has no field
                           order, drops a `json:"-"` field the wire still gives a
                           slot to, and flattens an embedded struct the wire
                           nests. So the Go SDK is rendered from the registry, by
                           zip, which owns that wire.

Everything downstream of the document reads the document's BYTES. `Emit` and
`EmitSpec` are therefore one pipeline reached two ways, not two implementations
to keep level — pinned by `TestTheDocumentIsTheOnlyIntermediateForm`, and
measured on the real egress service: the five spec-derived files are byte
identical whether generated in-process or from the written document.

## The two doors

In-process, for the whole surface including the Go SDK:

    opgen.Emit(app, opgen.Options{Dir: "sdk"})

From a document, for a service this binary does not link:

    service openapi openapi.json      # zip's own verb, App.Described()
    opgen -spec openapi.json -out sdk

The Go SDK is not reachable from a document and is refused rather than skipped:
it speaks a wire the document does not describe.

## What is written

    openapi.json                 the contract
    mcp.json                     the agent tool list
    commands.json                     the command tree
    go/<name>/<name>.go          Go client, ZAP wire, rendered by zip
    rust/<name>/                 Rust crate, HTTP/JSON
    cpp/include/<name>/          C++ header, HTTP/JSON

## Why the emitters are ours and not openapi-generator

Measured, on a document zip wrote for a struct holding every Go integer width.
openapi-generator 7.25.0, `-g rust`:

    zip says        openapi-generator      opgen
    int8            i32                    i8
    int16           i32                    i16
    int32           i32                    i32
    int64           i64                    i64
    uint8           i32                    u8
    uint16          i32                    u16
    uint32          i32                    u32
    uint64          i32                    u64
    float           f32                    f32

Six of nine wrong, and two of them cannot hold what the service sends. Fed
`{"h": 18446744073709551615}` — a `uint64` field — the generated `Option<i32>`
refuses to deserialize: *invalid value: integer 18446744073709551615, expected
i32*. Six of the eight integer formats zip emits are the Go spelling rather than
OpenAPI's four standard ones, and a generic generator has no reason to know
them.

It also costs 16 files and 296 lines where this costs 2 and 110, drags in
reqwest and a TLS stack, and needs a JVM in CI. And it cannot reach the ZAP
wire at all, so the Go leg would still have to come from somewhere else — which
is two pipelines again, which is the thing being removed.

Credit where it is due: it gets the recursive type right, the same way.

## A service the size of a node is not one app

node runs nine zip apps, one per service — platform, admin, info, xvm, xsvm,
indexer, health, security, proposervm — 94 typed ops between them. A client that
had to be nine crates would be nine things to keep level, which is the problem
again in a new shape.

zip already composes: `host.Use(child)` makes every projection the union, and a
composed child's types are qualified by their origin, so two services that both
declare a Party are two types rather than a silent collision. Compose them and
run this once on the host.

## The docs leg

There is no docs emitter here and there should not be one. `openapi.json` IS the
docs source: `@hanzo/docs-openapi` — our own fumadocs fork, already what
docs.lux.network is built on — turns an OpenAPI document into MDX pages. Adding
a second renderer would be a second way to do what that package does.

    zip typed ops (Go)
      |
      +- openapi.json -+- @hanzo/docs-openapi -> MDX -> the docs site
      |                +- commands.json  (zip.CommandsFromSpec)
      |                +- rust/
      |                +- cpp/
      +- mcp.json      the agent tool list
      +- go/           zip.App.SDK, the ZAP wire

## What the SDKs depend on

Rust: serde and serde_json. C++: nlohmann/json, and C++20.

No HTTP client, no async runtime, no TLS. The SDK knows the contract; the
program it lands in already has an HTTP client, and a `Transport` is where it
plugs in. A generated SDK that chose one would be the reason a build has two TLS
stacks in it.

    trait Transport { fn send(&self, method, target, body) -> Result<Reply, Error> }

`target` is an absolute path with its query string, so the base address belongs
to the transport and not to the call.

## Things that were measured, not assumed

**zip leaks undeclared routes into its extraction calls.** A route registered
through `zip.Undeclared` serves without being part of the contract — a
retirement answering 410 is the case it exists for. zip's doc says such a route
is "in none of the projections". Measured against v1.36.38 it appears in
`OpenAPISpec()`, in `MCPTools()` and in `Commands()`, while the live `/mcp` door
filters it out. Publishing from the raw calls would put a dead address in the
docs site, in three SDKs and in the agent's tool list. Every projection here is
filtered through `App.Declares` first, and the schemas only those addresses
reached are pruned with them. `TestAnUndeclaredOpIsInNoProjection` skips itself
if zip ever stops leaking, so the filter cannot outlive its reason.

**The two wires do not carry the same ops.** A `map` or an `any` field crosses
the JSON edge and not the ZAP wire, so the Go SDK can legitimately have fewer
methods than the Rust and C++ ones. That is the wire refusing, not drift, and
`Result.Gaps` reports it per projection. The fixture's `vault_seal` is the case:
four ops over HTTP, three over ZAP.

**The document spells eight integer widths**, six of which are not standard
OpenAPI — `int8`, `int16`, `uint8`, `uint16`, `uint32`, `uint64` are the Go
spelling. Reading only `int32`/`int64` silently widens every unsigned field.

**A reply is not always under `"200"`.** `zip.WithStatus` lets an op declare its
own codes, so the lowest 2xx carrying a body is the answer.

**A type that reaches itself has no size** in Rust or C++, and the document
cannot say whether the Go field was a pointer — `*T` and `T` are both a `$ref`.
The fields on a cycle are marked and only those go behind `Option<Box<T>>` and
`std::shared_ptr<T>`.

**A declaration and a document do not spell a path the same way, and the two do
not invert.** zip renders both `*` and `+` as `{wildcard1}`, so reading a
document path back into a router pattern is a guess — and guessing wrong dropped
every wildcard route from the contract silently. The filter keys on the
operationId instead, which is one token in both places and the same one the tool,
the command and the SDK method are named after.

**An address the input type cannot fill gets no method.** A wildcard segment
names no Go field, so a client holding only the input value cannot spell the
address. The op stays in the document — the service answers there — and the
Rust and C++ clients report it as a gap rather than emitting a method that names
a field its own type has not got.

**Nothing unused is generated.** A crate carrying a percent-encoder no address
calls does not compile under the dead-code lint. Found by generating the real
egress client, whose three ops all answer at fixed addresses.

**An ambiguous route panics inside fiber** rather than returning an error from
`Build()`, so `Emit` panics too. Deliberately not recovered: the stack trace
names the two conflicting routes, which is more than a wrapped error would.

## What is deliberately not carried

An anonymous struct field is inlined by the document with no name, so it becomes
a free JSON value rather than an invented type name. A `[]byte` is base64 text
on this edge and stays `String`/`std::string` rather than pulling a base64 codec
into two languages. A `validate:"required"` tag is a validation rule, not a
presence rule — every Go field is present with its zero value, so every
generated field is too, and none is `Option`.

## Testing

`make check`. The compile tests build the generated Rust, C++ and Go for real —
`cargo test` runs six integration tests against the generated crate, and the C++
header is compiled under `-Wall -Wextra -Werror` and run. They skip when a
toolchain is absent, so a green run says what it proved.

## CI

Two gates. Generate, then refuse a diff:

    go run ./cmd/opgen -spec openapi.json -out sdk
    git diff --exit-code sdk

The second line is what makes "never drifts" true rather than aspirational: a
merged op that nobody regenerated for fails the build.
