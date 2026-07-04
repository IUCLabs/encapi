// Package models holds the wire types shared between the encapi server, the
// encapi-cli client, and (via the generated client) the Terraform provider.
package models

// Role is a Puppet class assignment target, e.g. "roles::infra::storage::vault".
// DefaultParams are inheritable parameters merged into every node that carries
// the role; a node's own params take precedence on key collisions.
type Role struct {
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	DefaultParams map[string]any `json:"default_params,omitempty"`
}

// Status is a Puppet environment (Cobbler calls these "status": testing,
// production, development, ...). The set of valid statuses is managed
// explicitly so a node can only be pinned to one that exists.
type Status struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Node is a host-to-role assignment. Certname is the Puppet certname (fqdn).
// Params override the role's DefaultParams for this host only.
type Node struct {
	Certname    string         `json:"certname"`
	Role        string         `json:"role"`
	Environment string         `json:"environment"`
	Params      map[string]any `json:"params,omitempty"`
}
