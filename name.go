package opgen

// Spelling one name four ways.
//
// An operation has ONE identity, the operationId, and four languages that each
// insist on their own casing for it. Every rename happens here, from that one
// token, so a method in the Rust client and a method in the C++ client are the
// same word in two accents rather than two words.

import (
	"strings"
	"unicode"
)

// words splits an identifier however it was written — vault_seal, vaultSeal,
// vault-seal, vault.seal — into its words, lowercased.
func words(s string) []string {
	var out []string
	var cur strings.Builder
	var prev rune
	for i, r := range s {
		switch {
		case r == '_' || r == '-' || r == '.' || r == ' ' || r == '/':
			if cur.Len() > 0 {
				out = append(out, strings.ToLower(cur.String()))
				cur.Reset()
			}
		case i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(prev):
			if cur.Len() > 0 {
				out = append(out, strings.ToLower(cur.String()))
				cur.Reset()
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
		prev = r
	}
	if cur.Len() > 0 {
		out = append(out, strings.ToLower(cur.String()))
	}
	return out
}

// title is VaultSeal: the type name a language that names types that way wants.
func title(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(w[1:])
	}
	if b.Len() == 0 {
		return s
	}
	return b.String()
}

// snake is vault_seal: the method name Rust and C++ both want.
func snake(s string) string { return strings.Join(words(s), "_") }

// safe keeps an identifier out of a language's keyword set by suffixing it,
// which is the one rename that cannot collide with another field: a document
// that had both `type` and `type_` would already be two names.
func safe(name string, reserved map[string]bool) string {
	if reserved[name] {
		return name + "_"
	}
	return name
}
