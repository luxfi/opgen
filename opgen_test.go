package opgen_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxfi/opgen"
	"github.com/luxfi/opgen/internal/fixture"
	"github.com/zap-proto/zip"
)

func emit(t *testing.T, app *zip.App, o opgen.Options) (*opgen.Result, string) {
	t.Helper()
	if o.Dir == "" {
		o.Dir = t.TempDir()
	}
	r, err := opgen.Emit(app, o)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	return r, o.Dir
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// One run writes the whole surface. A generator that emitted some of it would
// leave the rest to a second pipeline, which is the drift this exists to stop.
func TestEveryProjectionIsWritten(t *testing.T) {
	r, dir := emit(t, fixture.App(), opgen.Options{})
	want := []string{
		"commands.json",
		"cpp/CMakeLists.txt",
		"cpp/include/vault/vault.hpp",
		"go/vault/vault.go",
		"mcp.json",
		"openapi.json",
		"rust/vault/Cargo.toml",
		"rust/vault/src/lib.rs",
	}
	if strings.Join(r.Files, ",") != strings.Join(want, ",") {
		t.Fatalf("wrote %v, want %v", r.Files, want)
	}
	for _, f := range want {
		if len(read(t, filepath.Join(dir, f))) == 0 {
			t.Errorf("%s is empty", f)
		}
	}
	if r.Ops != 4 {
		t.Errorf("Ops = %d, want the fixture's 4", r.Ops)
	}
}

// The document is the intermediate form, and everything derived from it reads
// its BYTES. So generating from a live app and generating from the document
// that app wrote are not two implementations to keep level — they are one, and
// this says so in the only way that stays true: by comparing the output.
func TestTheDocumentIsTheOnlyIntermediateForm(t *testing.T) {
	_, live := emit(t, fixture.App(), opgen.Options{})

	fromDoc := t.TempDir()
	if _, err := opgen.EmitSpec(read(t, filepath.Join(live, "openapi.json")), opgen.Options{Dir: fromDoc, Name: "vault"}); err != nil {
		t.Fatalf("EmitSpec: %v", err)
	}
	for _, f := range []string{"commands.json", "cpp/include/vault/vault.hpp", "cpp/CMakeLists.txt", "rust/vault/src/lib.rs", "rust/vault/Cargo.toml"} {
		if !bytes.Equal(read(t, filepath.Join(live, f)), read(t, filepath.Join(fromDoc, f))) {
			t.Errorf("%s differs between the app and its own document", f)
		}
	}
}

// Running twice writes the same bytes. A generator whose output moved would
// show a diff on every CI run and teach everyone to ignore the diff.
func TestTheSameAppWritesTheSameBytes(t *testing.T) {
	_, one := emit(t, fixture.App(), opgen.Options{})
	_, two := emit(t, fixture.App(), opgen.Options{})
	for _, f := range []string{"openapi.json", "mcp.json", "commands.json", "go/vault/vault.go", "rust/vault/src/lib.rs", "cpp/include/vault/vault.hpp"} {
		if !bytes.Equal(read(t, filepath.Join(one, f)), read(t, filepath.Join(two, f))) {
			t.Errorf("%s is not reproducible", f)
		}
	}
}

type hidden struct {
	A string `json:"a"`
}

func gone(_ context.Context, in *hidden) (*hidden, error) { return in, nil }

// A route registered through zip.Undeclared serves without being part of the
// contract. zip's extraction calls do not honour that — measured: the op is in
// OpenAPISpec, in MCPTools and in Commands, while the live MCP door filters it
// out. Publishing from the raw calls puts a dead address in the docs site, in
// three SDKs and in the agent's tool list.
func TestAnUndeclaredOpIsInNoProjection(t *testing.T) {
	app := fixture.App()
	zip.Post(zip.Undeclared(app), "/v1/retired", gone,
		zip.WithOperationID("vault_retired"),
		zip.WithSummary("gone"))

	// First, that the leak is real and this test is not vacuous.
	if err := app.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw := false
	for _, tool := range app.MCPTools() {
		if tool["name"] == "vault_retired" {
			raw = true
		}
	}
	if !raw {
		t.Skip("zip no longer leaks undeclared ops into MCPTools; the filter is now redundant")
	}

	_, dir := emit(t, app, opgen.Options{})
	for _, f := range []string{"openapi.json", "mcp.json", "commands.json", "rust/vault/src/lib.rs", "cpp/include/vault/vault.hpp"} {
		if body := read(t, filepath.Join(dir, f)); bytes.Contains(body, []byte("vault_retired")) || bytes.Contains(body, []byte("/v1/retired")) {
			t.Errorf("%s publishes an undeclared address", f)
		}
	}
	// And the schema it was the only user of goes with it.
	if bytes.Contains(read(t, filepath.Join(dir, "openapi.json")), []byte("\"hidden\"")) {
		t.Error("openapi.json keeps a schema no published op reaches")
	}
}

// The two wires do not carry the same set of operations, and the run says so
// rather than letting a caller assume every client has every method. vault_seal
// takes a map and an `any`, neither of which the ZAP wire can express, so the
// Go SDK has three methods where the HTTP clients have four.
func TestAWireThatCannotCarryAnOpSaysSo(t *testing.T) {
	r, dir := emit(t, fixture.App(), opgen.Options{})
	if len(r.Gaps) != 1 || r.Gaps[0].Op != "vault_seal" || r.Gaps[0].Where != opgen.Go {
		t.Fatalf("Gaps = %+v, want the ZAP wire refusing vault_seal", r.Gaps)
	}
	if bytes.Contains(read(t, filepath.Join(dir, "go/vault/vault.go")), []byte("VaultSeal")) {
		t.Error("the Go SDK has a method for an op its wire refuses")
	}
	for _, f := range []string{"rust/vault/src/lib.rs", "cpp/include/vault/vault.hpp"} {
		if !bytes.Contains(read(t, filepath.Join(dir, f)), []byte("vault_seal")) {
			t.Errorf("%s is missing an op the JSON edge carries fine", f)
		}
	}
}

// The type matrix, read back out of the document.
func TestTheDocumentCarriesEveryKind(t *testing.T) {
	_, dir := emit(t, fixture.App(), opgen.Options{})
	s, err := opgen.Read(read(t, filepath.Join(dir, "openapi.json")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var seal opgen.Type
	for _, ty := range s.Types {
		if ty.Name == "Seal" {
			seal = ty
		}
	}
	want := map[string]opgen.Kind{
		"name":   {Sort: opgen.Text},
		"bytes":  {Sort: opgen.Whole, Bits: 64, Signed: true},
		"weight": {Sort: opgen.Real, Bits: 64},
		"rotate": {Sort: opgen.Flag},
		"blob":   {Sort: opgen.Octets},
		"at":     {Sort: opgen.Moment},
		"free":   {Sort: opgen.Free},
		"nested": {Sort: opgen.Named, Ref: "Party"},
		"owner":  {Sort: opgen.Named, Ref: "Party"},
	}
	got := map[string]opgen.Kind{}
	for _, f := range seal.Fields {
		got[f.Name] = f.Kind
	}
	for name, k := range want {
		if got[name] != k {
			t.Errorf("Seal.%s = %+v, want %+v", name, got[name], k)
		}
	}
	if k := got["tags"]; k.Sort != opgen.List || k.Elem.Sort != opgen.Text {
		t.Errorf("Seal.tags = %+v, want a list of text", k)
	}
	if k := got["counts"]; k.Sort != opgen.List || k.Elem.Sort != opgen.Whole || k.Elem.Bits != 32 {
		t.Errorf("Seal.counts = %+v, want a list of 32-bit whole numbers", k)
	}
	if k := got["labels"]; k.Sort != opgen.Table || k.Elem.Sort != opgen.Text {
		t.Errorf("Seal.labels = %+v, want a table of text", k)
	}
	// A field the Go type opts out of with json:"-" is on no wire at all.
	if _, leaked := got["Skipped"]; leaked {
		t.Error("a field marked json:\"-\" reached the document")
	}
	if len(seal.Fields) != len(want)+3 {
		t.Errorf("Seal has %d fields, want %d", len(seal.Fields), len(want)+3)
	}
}

// A bodyless op's input is one value on the Go side and a parameter list in the
// document. Putting it back together is what keeps every generated method the
// same shape.
func TestABodylessOpGetsItsInputBack(t *testing.T) {
	_, dir := emit(t, fixture.App(), opgen.Options{})
	s, err := opgen.Read(read(t, filepath.Join(dir, "openapi.json")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, o := range s.Ops {
		if o.ID != "vault_read" {
			continue
		}
		if o.In != "VaultRead" || o.Out != "Secret" {
			t.Fatalf("vault_read is %q -> %q, want VaultRead -> Secret", o.In, o.Out)
		}
		if strings.Join(o.Segments, ",") != "name" {
			t.Errorf("segments = %v, want the one path parameter", o.Segments)
		}
		if strings.Join(o.Query, ",") != "reveal" {
			t.Errorf("query = %v, want the one query parameter", o.Query)
		}
		if o.Body {
			t.Error("a GET was given a body")
		}
		return
	}
	t.Fatal("vault_read is not in the surface")
}

// Nothing is generated for a program that does not build. An app that fails to
// build projects as an EMPTY one, so without this a release writes an empty
// document and three empty SDKs for a broken service and exits 0.
func TestABrokenAppIsNotDescribed(t *testing.T) {
	app := zip.New(zip.Config{AppName: "broken", DisableStartupMessage: true})
	zip.Get(app, "/v1/one", gone, zip.WithOperationID("same"))
	zip.Get(app, "/v1/two", gone, zip.WithOperationID("same"))

	dir := t.TempDir()
	if _, err := opgen.Emit(app, opgen.Options{Dir: dir}); err == nil {
		t.Fatal("Emit described an app that does not build")
	}
	if names, _ := os.ReadDir(dir); len(names) != 0 {
		t.Errorf("wrote %d files for a broken app, want none", len(names))
	}
}

// A document is enough for the client legs and is NOT enough for the Go SDK,
// which speaks a wire the document does not describe. Asking for one from a
// document is refused rather than quietly skipped.
func TestTheGoSDKIsNotReachableFromADocument(t *testing.T) {
	_, dir := emit(t, fixture.App(), opgen.Options{})
	out := t.TempDir()
	_, err := opgen.EmitSpec(read(t, filepath.Join(dir, "openapi.json")), opgen.Options{Dir: out, Name: "vault", Only: []zip.Projection{opgen.Go}})
	if err == nil {
		t.Fatal("EmitSpec rendered a Go SDK from a document")
	}
	if names, _ := os.ReadDir(out); len(names) != 0 {
		t.Errorf("wrote %d files, want none", len(names))
	}
}

// The MCP manifest is the tool list an agent reads, so its names have to be the
// operation ids every other projection uses.
func TestTheToolListNamesTheOperations(t *testing.T) {
	_, dir := emit(t, fixture.App(), opgen.Options{})
	var tools []map[string]any
	if err := json.Unmarshal(read(t, filepath.Join(dir, "mcp.json")), &tools); err != nil {
		t.Fatalf("mcp.json: %v", err)
	}
	var names []string
	for _, tool := range tools {
		names = append(names, tool["name"].(string))
		if _, ok := tool["inputSchema"]; !ok {
			t.Errorf("%v has no inputSchema", tool["name"])
		}
	}
	if strings.Join(names, ",") != "vault_health,vault_list,vault_read,vault_seal" {
		t.Errorf("tools = %v, want the four ops in name order", names)
	}
}

// ---- the generated code is compiled, not merely written ---------------------

// Rust: the crate builds, warnings are errors, and one call over a transport
// that answers from memory produces the address and body the service expects.
func TestTheRustCrateCompilesAndCalls(t *testing.T) {
	cargo := tool(t, "cargo")
	_, dir := emit(t, fixture.App(), opgen.Options{})
	crate := filepath.Join(dir, "rust", "vault")

	if err := os.MkdirAll(filepath.Join(crate, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crate, "tests", "calls.rs"), []byte(rustCheck), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, crate, cargo, "test", "--offline")
}

// C++: the header compiles under -Wall -Wextra -Werror and the same call
// produces the same address and body.
func TestTheCppHeaderCompilesAndCalls(t *testing.T) {
	compiler := cxx(t)
	_, dir := emit(t, fixture.App(), opgen.Options{})
	work := t.TempDir()
	src := filepath.Join(work, "calls.cpp")
	if err := os.WriteFile(src, []byte(cppCheck), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "calls")
	run(t, work, compiler, "-std=c++20", "-Wall", "-Wextra", "-Werror",
		"-I", filepath.Join(dir, "cpp", "include"), src, "-o", bin)
	run(t, work, bin)
}

// The Go SDK zip renders is compiled too, in a module of its own, because a
// generated package that does not build is the one failure the whole pipeline
// exists to prevent.
func TestTheGoSDKCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling the Go SDK needs the module cache")
	}
	_, dir := emit(t, fixture.App(), opgen.Options{})
	pkg := filepath.Join(dir, "go", "vault")
	mod := "module example.com/vaultsdk\n\ngo 1.26.5\n\nrequire github.com/zap-proto/zip " + zipVersion(t) + "\n"
	if err := os.WriteFile(filepath.Join(pkg, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, pkg, "go", "mod", "tidy")
	run(t, pkg, "go", "build", "./...")
}

// zipVersion is the zip this test binary was built against, so the generated
// SDK is compiled against the same one that rendered it.
func zipVersion(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "github.com/zap-proto/zip").Output()
	if err != nil {
		t.Skipf("cannot read the zip version: %v", err)
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		t.Skipf("cannot read the zip version from %q", out)
	}
	return parts[1]
}

// cargoBuild builds a generated crate. It prefers the cached registry so the
// check runs without a network, and falls back to fetching when the cache is
// cold — a proof that only runs on the machine that wrote it proves nothing.
func cargoBuild(t *testing.T, dir string, args ...string) {
	t.Helper()
	cargo := tool(t, "cargo")
	if _, err := attempt(t, dir, cargo, append(args, "--offline")...); err == nil {
		return
	}
	if out, err := attempt(t, dir, cargo, args...); err != nil {
		t.Fatalf("cargo %s:\n%s", strings.Join(args, " "), out)
	}
}

// cxx returns a C++ compiler that can see nlohmann/json, or skips. Asking the
// compiler is the only honest test: the header's location is the system's
// business, not ours.
func cxx(t *testing.T) string {
	t.Helper()
	compiler := tool(t, "g++")
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.cpp")
	if err := os.WriteFile(probe, []byte("#include <nlohmann/json.hpp>\nint main() { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := attempt(t, dir, compiler, "-std=c++20", probe, "-o", filepath.Join(dir, "probe")); err != nil {
		t.Skip("nlohmann/json is not where this compiler looks")
	}
	return compiler
}

func attempt(t *testing.T, dir, name string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+os.Getenv("HOME")+"/.cargo/bin:"+os.Getenv("PATH"), "RUSTFLAGS=-D warnings")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Logf("%s %s\n%s", filepath.Base(name), strings.Join(args, " "), out)
	}
	return out, err
}

func tool(t *testing.T, name string) string {
	t.Helper()
	if testing.Short() {
		t.Skipf("compiling %s output is not a short test", name)
	}
	path, err := exec.LookPath(name)
	if err != nil {
		for _, dir := range []string{os.Getenv("HOME") + "/.cargo/bin"} {
			if p := filepath.Join(dir, name); fileExists(p) {
				return p
			}
		}
		t.Skipf("%s is not installed", name)
	}
	return path
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	if out, err := attempt(t, dir, name, args...); err != nil {
		t.Fatalf("%s %s:\n%s", name, strings.Join(args, " "), out)
	}
}

type sealed struct {
	Key string `json:"key"`
}

func onlyBody(_ context.Context, in *sealed) (*sealed, error) { return in, nil }

// A service whose every address is fixed needs no percent-encoder, and a crate
// carrying one nothing calls does not compile under the dead-code lint. Found
// by generating the real egress client, whose three ops all answer at fixed
// addresses.
func TestNothingUnusedIsGenerated(t *testing.T) {
	app := zip.New(zip.Config{AppName: "fixed", DisableStartupMessage: true})
	zip.Post(app, "/v1/seal", onlyBody, zip.WithOperationID("fixed_seal"))

	_, dir := emit(t, app, opgen.Options{})
	crate := read(t, filepath.Join(dir, "rust/fixed/src/lib.rs"))
	if bytes.Contains(crate, []byte("fn encode(")) {
		t.Error("the crate carries an encoder no address uses")
	}

	// And the fixture, whose addresses do carry values, still has one.
	_, withValues := emit(t, fixture.App(), opgen.Options{})
	if !bytes.Contains(read(t, filepath.Join(withValues, "rust/vault/src/lib.rs")), []byte("fn encode(")) {
		t.Error("a service with values in its URLs lost its encoder")
	}

	// The compiler is the real check.
	cargoBuild(t, filepath.Join(dir, "rust", "fixed"), "build")
}

// tree reaches itself three ways: directly through a pointer, indirectly
// through a slice, and around through another type.
type tree struct {
	Name     string  `json:"name"`
	Parent   *tree   `json:"parent"`
	Children []tree  `json:"children"`
	Grove    *forest `json:"grove"`
}

type forest struct {
	Back *tree `json:"back"`
}

func plant(_ context.Context, in *tree) (*tree, error) { return in, nil }

// The document cannot say whether a Go field was a pointer — `*T` and `T` are
// both a $ref — so a type that reaches itself reads as one that contains itself
// by value, which neither Rust nor C++ can lay out. The fields on a cycle go
// behind a pointer and only those; a slice is already indirect and does not.
func TestATypeThatReachesItselfStillCompiles(t *testing.T) {
	app := zip.New(zip.Config{AppName: "grove", DisableStartupMessage: true})
	zip.Post(app, "/v1/plant", plant, zip.WithOperationID("grove_plant"))

	_, dir := emit(t, app, opgen.Options{})

	s, err := opgen.Read(read(t, filepath.Join(dir, "openapi.json")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// The invariant is not WHICH edge is broken — a cycle can be broken
	// anywhere on it, and the walk breaks it at the first back edge it finds.
	// The invariant is that what is left contains no type by value that
	// contains itself, because that is what has no size.
	if loop := byValue(s.Types); loop != "" {
		t.Errorf("%s still contains itself by value", loop)
	}
	cyclic := map[string]bool{}
	for _, ty := range s.Types {
		for _, f := range ty.Fields {
			if f.Cyclic {
				cyclic[ty.Name+"."+f.Name] = true
			}
		}
	}
	// A direct self-reference has one edge, so there is nothing to choose.
	if !cyclic["tree.parent"] {
		t.Error("tree.parent points at a tree and was not marked")
	}
	// A slice holds its elements elsewhere: already indirect, needs no pointer.
	if cyclic["tree.children"] {
		t.Error("a slice was given a pointer it does not need")
	}

	crate := string(read(t, filepath.Join(dir, "rust/grove/src/lib.rs")))
	if !strings.Contains(crate, "pub parent: Option<Box<Tree>>") {
		t.Error("the Rust field on the cycle is not behind a pointer")
	}
	if !strings.Contains(crate, "pub children: Vec<Tree>") {
		t.Error("the Rust slice was given a pointer it does not need")
	}
	header := string(read(t, filepath.Join(dir, "cpp/include/grove/grove.hpp")))
	if !strings.Contains(header, "std::shared_ptr<Tree> parent{};") {
		t.Error("the C++ field on the cycle is not behind a pointer")
	}
	if !strings.Contains(header, "std::vector<Tree> children{};") {
		t.Error("the C++ slice was given a pointer it does not need")
	}

	// The compilers are the real check.
	cargoBuild(t, filepath.Join(dir, "rust", "grove"), "build")

	compiler := cxx(t)
	work := t.TempDir()
	src := filepath.Join(work, "grove.cpp")
	if err := os.WriteFile(src, []byte(groveCheck), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, compiler, "-std=c++20", "-Wall", "-Wextra", "-Werror",
		"-I", filepath.Join(dir, "cpp", "include"), src, "-o", filepath.Join(work, "grove"))
	run(t, work, filepath.Join(work, "grove"))
}

// byValue names a type that still reaches itself through fields laid out by
// value, or "" when none does.
func byValue(types []opgen.Type) string {
	fields := map[string][]opgen.Field{}
	for _, t := range types {
		fields[t.Name] = t.Fields
	}
	const (
		fresh = iota
		open
		done
	)
	state := map[string]int{}
	var walk func(name string) bool
	walk = func(name string) bool {
		switch state[name] {
		case open:
			return true
		case done:
			return false
		}
		state[name] = open
		for _, f := range fields[name] {
			if f.Cyclic || f.Kind.Sort != opgen.Named {
				continue
			}
			if walk(f.Kind.Ref) {
				return true
			}
		}
		state[name] = done
		return false
	}
	for _, t := range types {
		if walk(t.Name) {
			return t.Name
		}
	}
	return ""
}

type payload struct {
	Body string `json:"body"`
}

func upload(_ context.Context, in *payload) (*payload, error) { return in, nil }

// A wildcard segment names no field, so a client holding only the input value
// cannot spell the address. It gets no method rather than one that names a
// field its own type has not got — the same rule the ZAP wire follows when it
// refuses a type, and for the same reason: a client that compiles and lies is
// worse than one that says the operation is not reachable.
func TestAnAddressTheInputCannotFillGetsNoMethod(t *testing.T) {
	app := zip.New(zip.Config{AppName: "store", DisableStartupMessage: true})
	zip.Post(app, "/v1/files/*", upload, zip.WithOperationID("store_upload"))
	zip.Post(app, "/v1/plain", upload, zip.WithOperationID("store_plain"))

	r, dir := emit(t, app, opgen.Options{})

	var where []string
	for _, g := range r.Gaps {
		if g.Op == "store_upload" {
			where = append(where, string(g.Where))
		}
	}
	if strings.Join(where, ",") != "cpp,rust" {
		t.Fatalf("gaps for store_upload = %v, want both HTTP clients", where)
	}
	for _, f := range []string{"rust/store/src/lib.rs", "cpp/include/store/store.hpp"} {
		body := read(t, filepath.Join(dir, f))
		if bytes.Contains(body, []byte("store_upload")) {
			t.Errorf("%s has a method for an address it cannot spell", f)
		}
		if !bytes.Contains(body, []byte("store_plain")) {
			t.Errorf("%s lost an op it can reach", f)
		}
	}
	// The address is still in the contract: the service answers there, and a
	// caller building the URL by hand needs to know it exists.
	if !bytes.Contains(read(t, filepath.Join(dir, "openapi.json")), []byte("store_upload")) {
		t.Error("the document dropped an address the service answers")
	}

	cargoBuild(t, filepath.Join(dir, "rust", "store"), "build")

	compiler := cxx(t)
	work := t.TempDir()
	src := filepath.Join(work, "store.cpp")
	if err := os.WriteFile(src, []byte("#include <store/store.hpp>\nint main() { return 0; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, work, compiler, "-std=c++20", "-Wall", "-Wextra", "-Werror",
		"-I", filepath.Join(dir, "cpp", "include"), src, "-o", filepath.Join(work, "store"))
}

// A run puts nothing down until all of it is rendered. A generator that wrote
// as it went would leave a document and a tool list behind when the SDK after
// them would not render — a half-described service, on disk, that reads as a
// whole one.
func TestAFailedRunLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	// A name no package, crate and namespace can share is refused after the
	// document is in hand and before anything is written.
	if _, err := opgen.Emit(fixture.App(), opgen.Options{Dir: dir, Name: "Not An Identifier"}); err == nil {
		t.Fatal("Emit accepted a name three languages cannot share")
	}
	if names, _ := os.ReadDir(dir); len(names) != 0 {
		t.Errorf("wrote %d files for a refused run, want none", len(names))
	}
}
