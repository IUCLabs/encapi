// Package distro resolves per-host provisioning parameters (e.g. epel version,
// operating system release) from an external kickstart/distro API.
//
// This is the seam that will let encapi take over the provisioning-param half
// of Cobbler's old ENC output. Today those params (epel, tree,
// operatingsystemrelease, from_cobbler) are unused by the Puppet manifests, so
// the resolver is OFF by default: with no API configured, Resolve returns nil
// and no distro params are injected.
package distro

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Resolver returns provisioning parameters for a host, or nil if none apply.
type Resolver interface {
	Resolve(ctx context.Context, certname string) (map[string]any, error)
}

// Noop is the default resolver: it injects nothing.
type Noop struct{}

// Resolve always returns nil.
func (Noop) Resolve(context.Context, string) (map[string]any, error) { return nil, nil }

// HTTPResolver queries an external distro API of the form
// GET {BaseURL}/{certname} -> {"params": {...}} (or a bare JSON object).
type HTTPResolver struct {
	BaseURL string
	Client  *http.Client
}

// New returns a Noop resolver when baseURL is empty, otherwise an HTTPResolver.
func New(baseURL string) Resolver {
	if baseURL == "" {
		return Noop{}
	}
	return &HTTPResolver{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Resolve fetches provisioning params for certname. A 404 means "no params for
// this host" and yields nil, nil rather than an error, so ENC rendering never
// fails just because a host is unknown to the distro API.
func (h *HTTPResolver) Resolve(ctx context.Context, certname string) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/%s", h.BaseURL, url.PathEscape(certname))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("distro api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distro api returned HTTP %d for %q", resp.StatusCode, certname)
	}

	// Accept either {"params": {...}} or a bare {...} object.
	var wrapper struct {
		Params map[string]any `json:"params"`
	}
	dec := json.NewDecoder(resp.Body)
	raw := map[string]any{}
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode distro api response: %w", err)
	}
	if p, ok := raw["params"].(map[string]any); ok {
		wrapper.Params = p
	} else {
		wrapper.Params = raw
	}
	return wrapper.Params, nil
}
