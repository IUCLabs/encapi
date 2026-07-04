// Package enc renders the External Node Classifier documents Puppet consumes.
//
// Two shapes are produced from the same node/role data:
//
//   - Final(): the reshaped document the exec node_terminus expects — classes
//     as a LIST, environment omitted when it equals "testing", and
//     parameters carrying enc_role (list) + enc_env. This mirrors the old
//     /opt/cobbler-enc/cobbler-enc wrapper output.
//   - Cobbler(): the raw cobbler-wire form — classes as a MAP keyed by role,
//     environment always present. This mirrors what
//     cobbler's /cblr/svc/op/puppet/hostname/<host> returned, so the
//     enc_direct_facts.rb fact (which reads classes.keys.first + environment)
//     keeps working by only swapping its base URL.
package enc

import (
	"gopkg.in/yaml.v3"

	"github.com/IUCLabs/encapi/pkg/models"
)

// TestingEnvironment is the sentinel environment that Puppet leaves implicit:
// when a node's environment is "testing" the key is dropped from ENC output so
// the agent falls back to its configured default environment.
const TestingEnvironment = "testing"

// mergeParams builds the effective parameter set for a node. Precedence,
// lowest to highest: distro-provided params, role default params, node params.
// The returned map is always non-nil.
func mergeParams(node models.Node, role models.Role, distro map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range distro {
		out[k] = v
	}
	for k, v := range role.DefaultParams {
		out[k] = v
	}
	for k, v := range node.Params {
		out[k] = v
	}
	return out
}

// Final renders the reshaped ENC document (see package doc) as YAML.
func Final(node models.Node, role models.Role, distro map[string]any) ([]byte, error) {
	params := mergeParams(node, role, distro)
	// enc_role and enc_env are authoritative and computed; set them last so
	// user params can never shadow them.
	params["enc_role"] = []string{node.Role}
	params["enc_env"] = node.Environment

	doc := map[string]any{
		"classes":    []string{node.Role},
		"parameters": params,
	}
	if node.Environment != TestingEnvironment {
		doc["environment"] = node.Environment
	}
	return yaml.Marshal(doc)
}

// Cobbler renders the cobbler-wire-compatible ENC document as YAML.
func Cobbler(node models.Node, role models.Role, distro map[string]any) ([]byte, error) {
	params := mergeParams(node, role, distro)

	// classes is a map keyed by role name; the value is the role's params so
	// class-scoped parameters survive for callers that consume them.
	classParams := map[string]any{}
	for k, v := range role.DefaultParams {
		classParams[k] = v
	}
	for k, v := range node.Params {
		classParams[k] = v
	}

	doc := map[string]any{
		"classes":     map[string]any{node.Role: classParams},
		"environment": node.Environment,
		"parameters":  params,
	}
	return yaml.Marshal(doc)
}
