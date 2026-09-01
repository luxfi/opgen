// Code generated from the typed-op registry by zip. DO NOT EDIT.

// Package vault calls vault's operations over ZAP.
package vault

import (
	"context"

	"github.com/zap-proto/zip"
)

type Ready struct {
	Ready bool   `json:"ready"`
	Name  string `json:"name"`
}

type Filter struct {
	Org   string `json:"org"`
	Limit int    `json:"limit"`
}

type Party struct {
	Org  string `json:"org"`
	Kind string `json:"kind"`
}

type Time struct {
}

type Secret struct {
	Name  string `json:"name"`
	Owner Party  `json:"owner"`
	At    Time   `json:"at"`
}

type Held struct {
	Secrets []Secret `json:"secrets"`
	More    bool     `json:"more"`
}

type Ref struct {
	Name   string `json:"name"`
	Reveal bool   `json:"reveal"`
}

// Client calls this service's operations. It is safe for concurrent use and
// holds a pooled connection, so a warm Client costs a round trip and no dial.
type Client struct{ conn *zip.Conn }

// Dial returns a Client for the service at addr. The scheme selects the
// transport exactly as it does everywhere else — a bare path is ZAP over a unix
// socket and a bare host:port is ZAP over tcp.
func Dial(addr string) (*Client, error) {
	conn, err := zip.Dial(addr)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// Open returns a Client over a connection the caller already holds, so one
// process reaching several services opens one pool rather than one per package.
func Open(conn *zip.Conn) *Client { return &Client{conn: conn} }

// Conn is the connection this Client calls over.
func (c *Client) Conn() *zip.Conn { return c.conn }

// Close releases the pooled connections. The Client stays usable; a later call
// redials.
func (c *Client) Close() error { return c.conn.Close() }

// VaultHealth calls vault_health.
func (c *Client) VaultHealth(ctx context.Context) (*Ready, error) {
	return zip.Call[struct{}, Ready](ctx, c.conn, "vault_health", (*struct{})(nil))
}

// VaultList calls vault_list.
func (c *Client) VaultList(ctx context.Context, in *Filter) (*Held, error) {
	return zip.Call[Filter, Held](ctx, c.conn, "vault_list", in)
}

// VaultRead calls vault_read.
func (c *Client) VaultRead(ctx context.Context, in *Ref) (*Secret, error) {
	return zip.Call[Ref, Secret](ctx, c.conn, "vault_read", in)
}
