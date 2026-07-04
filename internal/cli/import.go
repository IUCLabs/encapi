package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/IUCLabs/encapi/pkg/client"
	"github.com/IUCLabs/encapi/pkg/models"
)

// cobblerENC is the raw document Cobbler serves at
// /cblr/svc/op/puppet/hostname/<host>.
type cobblerENC struct {
	Classes     map[string]map[string]any `yaml:"classes"`
	Environment string                    `yaml:"environment"`
	Parameters  map[string]any            `yaml:"parameters"`
}

// importCobbler seeds encapi from the live Cobbler estate: it lists hosts from
// PuppetDB, reads each host's Cobbler ENC, and upserts the derived status,
// role, and node. It is a one-shot migration aid.
func importCobbler(ctx context.Context, c *client.Client, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import-cobbler", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cobblerURL := fs.String("cobbler-url", "http://cobbler.int.example.net", "Cobbler base URL")
	puppetdbURL := fs.String("puppetdb-url", "http://puppetdb.example.com:8080", "PuppetDB base URL")
	dryRun := fs.Bool("dry-run", false, "print actions without writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	hc := &http.Client{Timeout: 15 * time.Second}
	hosts, err := puppetdbNodes(ctx, hc, *puppetdbURL)
	if err != nil {
		return fail(stderr, fmt.Errorf("enumerate PuppetDB nodes: %w", err))
	}
	fmt.Fprintf(stderr, "found %d hosts in PuppetDB\n", len(hosts))

	seenStatus := map[string]bool{}
	seenRole := map[string]bool{}
	var imported, skipped int

	for _, host := range hosts {
		doc, err := cobblerLookup(ctx, hc, *cobblerURL, host)
		if err != nil {
			fmt.Fprintf(stderr, "skip %s: %v\n", host, err)
			skipped++
			continue
		}
		role := firstClass(doc.Classes)
		if role == "" {
			fmt.Fprintf(stderr, "skip %s: no class in Cobbler ENC\n", host)
			skipped++
			continue
		}
		env := doc.Environment
		if env == "" {
			env = "testing"
		}

		if *dryRun {
			fmt.Fprintf(stdout, "%s -> role=%s env=%s params=%v\n", host, role, env, doc.Classes[role])
			imported++
			continue
		}

		if !seenStatus[env] {
			if _, err := c.PutStatus(ctx, &models.Status{Name: env}); err != nil {
				return fail(stderr, fmt.Errorf("upsert status %q: %w", env, err))
			}
			seenStatus[env] = true
		}
		if !seenRole[role] {
			if _, err := c.PutRole(ctx, &models.Role{Name: role}); err != nil {
				return fail(stderr, fmt.Errorf("upsert role %q: %w", role, err))
			}
			seenRole[role] = true
		}
		params := doc.Classes[role]
		if len(params) == 0 {
			params = nil
		}
		if _, err := c.PutNode(ctx, &models.Node{Certname: host, Role: role, Environment: env, Params: params}); err != nil {
			return fail(stderr, fmt.Errorf("upsert node %q: %w", host, err))
		}
		imported++
	}
	fmt.Fprintf(stdout, "imported %d, skipped %d (%d roles, %d statuses)\n", imported, skipped, len(seenRole), len(seenStatus))
	return 0
}

// firstClass returns the sole/first class key deterministically.
func firstClass(classes map[string]map[string]any) string {
	keys := make([]string, 0, len(classes))
	for k := range classes {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	return keys[0]
}

func puppetdbNodes(ctx context.Context, hc *http.Client, base string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/pdb/query/v4/nodes", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var nodes []struct {
		Certname string `json:"certname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Certname)
	}
	sort.Strings(out)
	return out, nil
}

func cobblerLookup(ctx context.Context, hc *http.Client, base, host string) (*cobblerENC, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/cblr/svc/op/puppet/hostname/"+host, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cobbler HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var doc cobblerENC
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse cobbler yaml: %w", err)
	}
	return &doc, nil
}
