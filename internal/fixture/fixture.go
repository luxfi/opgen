// Package fixture is a zip app that exercises every shape a typed op can take,
// so opgen's emitters are tested against the whole matrix rather than the two
// types a hand-written example happens to use.
//
// It is a vault: it seals a secret, reads one back by name, lists what it holds
// and says whether it is up. Four ops, chosen because between them they cover a
// body op, a path parameter, a query parameter and an op that takes nothing.
package fixture

import (
	"context"
	"time"

	"github.com/zap-proto/zip"
)

// Seal is the body of a seal call, and the type matrix: every kind a generated
// client has to spell.
type Seal struct {
	Name    string            `json:"name"`
	Bytes   int64             `json:"bytes"`
	Weight  float64           `json:"weight"`
	Rotate  bool              `json:"rotate"`
	Blob    []byte            `json:"blob"`
	At      time.Time         `json:"at"`
	Tags    []string          `json:"tags"`
	Labels  map[string]string `json:"labels"`
	Owner   *Party            `json:"owner"`
	Nested  Party             `json:"nested"`
	Counts  []int32           `json:"counts"`
	Free    any               `json:"free"`
	Skipped string            `json:"-"`
}

// Party is a nested declared type, so the emitters have to name a schema and
// refer to it twice — once through a pointer and once by value.
type Party struct {
	Org  string `json:"org"`
	Kind string `json:"kind"`
}

// Sealed is what a seal answers.
type Sealed struct {
	Ref string    `json:"ref"`
	At  time.Time `json:"at"`
}

// Ref addresses one secret by name. Name is a path segment; Reveal rides the
// query string, because a GET has no body to carry it.
type Ref struct {
	Name   string `json:"name"`
	Reveal bool   `json:"reveal"`
}

// Secret is one held secret, without its value.
type Secret struct {
	Name  string    `json:"name"`
	Owner Party     `json:"owner"`
	At    time.Time `json:"at"`
}

// Filter narrows a list. Every field is a query parameter.
type Filter struct {
	Org   string `json:"org"`
	Limit int    `json:"limit"`
}

// Held is a page of secrets.
type Held struct {
	Secrets []Secret `json:"secrets"`
	More    bool     `json:"more"`
}

// Nothing is the empty input of an op that asks for nothing.
type Nothing struct{}

// Ready reports whether this replica serves.
type Ready struct {
	Ready bool   `json:"ready"`
	Name  string `json:"name"`
}

type vault struct{}

func (vault) seal(_ context.Context, in *Seal) (*Sealed, error) {
	return &Sealed{Ref: in.Name, At: in.At}, nil
}

func (vault) read(_ context.Context, in *Ref) (*Secret, error) {
	return &Secret{Name: in.Name}, nil
}

func (vault) list(_ context.Context, in *Filter) (*Held, error) {
	return &Held{}, nil
}

func (vault) health(_ context.Context, _ *Nothing) (*Ready, error) {
	return &Ready{Ready: true, Name: "vault"}, nil
}

// App builds the fixture. Every op is declared — an undeclared op is in no
// projection, and a fixture whose ops were invisible would test nothing.
func App() *zip.App {
	a := zip.New(zip.Config{AppName: "vault", DisableStartupMessage: true})
	v := vault{}
	zip.Post(a, "/v1/seal", v.seal,
		zip.WithOperationID("vault_seal"),
		zip.WithSummary("Seal a secret. The value is never readable again."))
	zip.Get(a, "/v1/secrets/:name", v.read,
		zip.WithOperationID("vault_read"),
		zip.WithSummary("Read one secret's record by name"))
	zip.Get(a, "/v1/secrets", v.list,
		zip.WithOperationID("vault_list"),
		zip.WithSummary("List the secrets this vault holds"))
	zip.Get(a, "/v1/health", v.health,
		zip.WithOperationID("vault_health"),
		zip.WithSummary("Report whether this replica is serving"))
	return a
}
