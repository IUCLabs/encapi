// Package migrations embeds encapi's SQL schema so the server carries it in
// the binary and applies it at startup, with no separately deployed copy to
// drift out of sync.
package migrations

import "embed"

// FS holds every numbered migration; lexical filename order is version order.
//
//go:embed *.sql
var FS embed.FS

// LockName names the cluster-wide advisory lock replicas contend for while
// migrating. golib derives the key as FNV-1a/64 of this name, so every replica
// migrating this database must agree on the string.
const LockName = "encapi-migrations"
