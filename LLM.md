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
    commands.json                the command tree
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

Rust: serde and serde_json. C++: nlohmann/json, and C++17 — which is what the
header actually needs, an init-statement in an `if` and nothing later. Both g++
and clang++ compile it under `-Wall -Wextra -Werror`.

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

**zip did not decode a path parameter, and a generated client found it.** Every
client percent-encodes a path segment, because a space, a slash or a percent has
no other spelling between two slashes. The router matched on the raw text and
handed that same text to the handler, so a client addressed at
`/v1/secrets/café` had its value arrive as `caf%C3%A9` and no path parameter
carrying such a character could round-trip. Fixed upstream in zip v1.36.39; the
end-to-end check here is what caught it, and an in-memory transport never could
have.

**A zip service speaks ZAP unless told otherwise.** The address scheme picks the
wire, and a bare `host:port` is ZAP over tcp. The Rust and C++ clients are HTTP
clients, so they reach a service listening on `http://`.

**A struct with no fields has a writer that never touches the value**, which C++
calls an unused parameter and `-Werror` calls an error. Found on node's admin
service, whose `EmptyReply` answers eight of its eighteen ops.

**A Go service spells its version with a leading v** and cargo will not parse a
manifest carrying one. node's platformvm declares `version.Current.String()` —
`v1.36.181` — which produced a crate that would not build. The document keeps
the service's own spelling; the manifest gets the one cargo accepts.

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
`cargo test` runs six integration tests against the generated crate, the C++
header is compiled under `-Wall -Wextra -Werror` and run, and one check starts
the fixture service on a real socket and calls it from the generated Rust client
through a hand-written HTTP transport of about thirty lines. That last one is
the only check that can say the service ACCEPTS what the client builds; the
others can only say the client builds what we meant. They skip when a
toolchain is absent, which is right on a laptop and wrong in CI: a job that
installed cargo and then skipped the checks that use it is a gate that runs
nothing. So CI installs all three toolchains and fails on any skip.

## The committed example

`example/` is the fixture service's whole generated surface, checked in. A
reader can see exactly what this produces without running it, and CI regenerates
it and refuses a diff, so a change to an emitter shows up as a change to the
code it emits in the same review. The generated Go SDK is a package of this
module, so `go build ./...` compiles it.

    go run ./internal/fixture/cmd -out example

## CI, and a fleet that is dark

`.github/workflows/ci.yml` targets `lux-build`, the self-hosted runner every
other luxfi repository uses. That fleet is not servicing this org: every `ci`
run in luxfi/crypto, luxfi/cli and luxfi/genesis since June sits queued until
GitHub cancels it at the 24-hour timeout, and the only green runs anywhere in
the org are GitHub's own dependency-graph jobs. So the workflow here is correct
and does not run, and `make check` is the gate that does.

Two gates. Generate, then refuse a diff:

    go run ./cmd/opgen -spec openapi.json -out sdk
    git diff --exit-code sdk

The second line is what makes "never drifts" true rather than aspirational: a
merged op that nobody regenerated for fails the build.

## Proven at the scale it is for

node's nine apps, 94 typed ops, generated and compiled:

    app          ops  types   go  gaps   cargo -D warnings   g++/clang -Werror
    platformvm    31     56   31     0          ok                  ok
    admin         18     56   17     1          ok                  ok
    info          14     21   14     0          ok                  ok
    xvm           10     21   10     0          ok                  ok
    xsvm           9     20    7     2          ok                  ok
    indexer        6     10    6     0          ok                  ok
    health         3      4    3     0          ok                  ok
    security       2      3    2     0          ok                  ok
    proposervm     1      1    1     0          ok                  ok

The Rust and C++ clients cover every one of the 94 ops and the Go SDK reaches
91. The three it does not are rooted in a value that has no wire form, rather
than in a type nobody had written a codec for: admin's `get_config` answers a
`map[string]interface{}`, and xsvm's `get_block` and `get_block_last` answer a
`tx.Tx`, which embeds the `Unsigned` interface. A map and an interface cross the
JSON edge and not the ZAP wire, so those three are the wire refusing, which is a
fact about the service and not about this.

THE INVARIANT, checked over all 94: every op has a Go method XOR a gap. Never
both, which was the op that compiled and lost its payload. Never neither, which
was the op that vanished with nothing recorded. Before the upstream fixes it
failed both ways at once.

## What this found upstream

Building a generator is a way of reading a framework very carefully. Three
defects in zip fell out of it, and one thing zip could not yet do. All four are
upstream and depended on here.

**A path parameter was not decoded** (zip v1.36.39). Every client percent-encodes
a path segment, because a space, a slash or a percent has no other spelling
between two slashes. The router matched on the raw text and handed that same
text to the handler, so a value addressed at `/v1/secrets/café` arrived as
`caf%C3%A9` and no such address could round-trip in any language. Found by the
end-to-end check here, and by nothing else — an in-memory transport cannot see
it.

**A refused type was written as an empty struct** (zip v1.36.40), in three
shapes, all found generating clients for node's ninety-four ops:

- A field whose own type had no wire form became `struct{}` and the op KEPT its
  method. The call succeeded and the payload was gone — the "compiles and lies"
  outcome the projection exists to prevent. node's admin, info and platformvm
  all shipped `[]struct{}` where a value should be.
- On an embedded field that substitute is not Go, so the package did not parse
  and `App.SDK` returned an error. Because the Go leg is built before anything
  is written, one such op cost that whole app its client surface: no document,
  no tool list, no Rust, no C++. One op of xvm's ten has that shape.
- The memo remembering a refused type answered "no" and said nothing, so the
  SECOND op reaching it had no method and no gap. platformvm's `get_tx_rewards`
  vanished exactly that way, and only counting methods would have shown it.

A refusal now travels up: a field that cannot cross refuses the type holding it,
which refuses the op, reported once at the field.

Re-run over node afterwards: xvm went from generating nothing to all eight
files; platformvm went from 14 Go methods to 10, and the four that left are
exactly the four that carried `[]struct{}`; `get_tx_rewards` is reported instead
of absent. The document, the tool list, the command tree and both HTTP clients
are byte-identical across the fix — only the Go leg moved, which is the only
leg that was wrong.

**The Go SDK had no codec to call through** (zip v1.36.41). A gap was the ZAP
wire refusing an id whose codec nobody had registered, and a service registers
codecs for the values it sends rather than for the reply shapes a client names,
so most of node's ops had none. zip now emits the codec beside the method —
`MarshalZAP` and `UnmarshalZAP` written against constant offsets — so the
generated SDK carries its own. Over the same 94 ops the Go leg went from 43
methods to 91, and the three that remain are the map and the interface above.
Six of the 72 generated files moved, all six the Go SDK of an app that gained
methods; the document, the tool list, the command tree and both HTTP clients are
byte-identical again.
