// Package cli implements the encapi-cli command tree. Logic lives here (rather
// than in main) so it can be unit-tested by driving Run with in-memory streams.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/IUCLabs/encapi/pkg/client"
)

// Env carries the process environment the CLI needs.
type Env struct {
	URL   string // ENCAPI_URL
	Token string // ENCAPI_WRITE_TOKEN
}

// LoadEnv reads configuration from the process environment, applying defaults.
func LoadEnv() Env {
	url := os.Getenv("ENCAPI_URL")
	if url == "" {
		url = "http://localhost:8000"
	}
	return Env{URL: strings.TrimRight(url, "/"), Token: os.Getenv("ENCAPI_WRITE_TOKEN")}
}

const usage = `encapi-cli — manage the Puppet External Node Classifier

Usage:
  encapi-cli classify <certname>              print the ENC document Puppet consumes
  encapi-cli node list
  encapi-cli node get <certname>
  encapi-cli node set <certname> --role <r> --env <e> [--param k=v ...]
  encapi-cli node set-env <certname> <environment>
  encapi-cli node restore-env <certname>      move a testing node back to its previous environment
  encapi-cli node delete <certname>
  encapi-cli role list
  encapi-cli role get <name>
  encapi-cli role set <name> [--desc <d>] [--param k=v ...]
  encapi-cli role delete <name>
  encapi-cli status list
  encapi-cli status get <name>
  encapi-cli status set <name> [--desc <d>]
  encapi-cli status delete <name>
  encapi-cli import-cobbler [--cobbler-url URL] [--puppetdb-url URL] [--dry-run]

Params:
  --param values are parsed as JSON when possible, so numbers, bools, lists and
  objects keep their type (replicas=3 -> int, enabled=true -> bool). To force a
  string, quote it: epel='"9"'.

Environment:
  ENCAPI_URL           encapi base URL (default http://localhost:8000)
  ENCAPI_WRITE_TOKEN   bearer token (static token or Vault identity JWT), required for writes
`

// Run executes the CLI and returns a process exit code.
func Run(args []string, env Env, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	c := client.New(env.URL, env.Token)
	ctx := context.Background()

	switch args[0] {
	case "classify":
		return classify(ctx, c, args[1:], stdout, stderr)
	case "node":
		return nodeCmd(ctx, c, args[1:], stdout, stderr)
	case "role":
		return roleCmd(ctx, c, args[1:], stdout, stderr)
	case "status":
		return statusCmd(ctx, c, args[1:], stdout, stderr)
	case "import-cobbler":
		return importCobbler(ctx, c, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func classify(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: encapi-cli classify <certname>")
		return 2
	}
	out, err := c.ENC(ctx, args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = stdout.Write(out)
	return 0
}

func printYAML(w io.Writer, v any) {
	b, _ := yaml.Marshal(v)
	_, _ = w.Write(b)
}

// parseParams turns ["k=v", "n=3"] into a map. Values are parsed as JSON when
// possible (so numbers, bools, lists, and objects survive), else kept as
// strings.
func parseParams(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --param %q (want key=value)", p)
		}
		var parsed any
		if json.Unmarshal([]byte(v), &parsed) == nil {
			out[k] = parsed
		} else {
			out[k] = v
		}
	}
	return out, nil
}

// leadingName splits a positional name from trailing flags. Go's flag package
// stops at the first non-flag token, so `set <name> --flag ...` needs the name
// peeled off first. Returns ok=false when no name is present.
func leadingName(args []string) (name string, rest []string, ok bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, false
	}
	return args[0], args[1:], true
}

// stringsFlag collects repeated flag values (e.g. multiple --param).
type stringsFlag []string

func (s *stringsFlag) String() string { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
