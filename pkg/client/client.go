// Package client is a Go SDK for the encapi HTTP API. It is used by encapi-cli
// and can be vendored by other Go callers (e.g. the Terraform provider).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/IUCLabs/encapi/pkg/models"
)

// Client talks to an encapi server. Token is required only for writes.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// New returns a Client for baseURL. Token may be empty for read-only use.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string { return fmt.Sprintf("encapi: HTTP %d: %s", e.Status, e.Msg) }

// NotFound reports whether err is a 404 from the API.
func NotFound(err error) bool {
	var ae *APIError
	if e, ok := err.(*APIError); ok {
		ae = e
	}
	return ae != nil && ae.Status == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg := decodeError(resp.Body)
		return &APIError{Status: resp.StatusCode, Msg: msg}
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func decodeError(r io.Reader) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(r).Decode(&e) == nil && e.Error != "" {
		return e.Error
	}
	return "request failed"
}

// ENC fetches the reshaped ENC document (YAML) Puppet's exec terminus consumes.
func (c *Client) ENC(ctx context.Context, certname string) ([]byte, error) {
	return c.getRaw(ctx, "/api/v1/nodes/"+url.PathEscape(certname)+"/enc")
}

// ENCCobbler fetches the cobbler-wire-compatible ENC document (YAML).
func (c *Client) ENCCobbler(ctx context.Context, certname string) ([]byte, error) {
	return c.getRaw(ctx, "/cblr/svc/op/puppet/hostname/"+url.PathEscape(certname))
}

func (c *Client) getRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Msg: decodeError(resp.Body)}
	}
	return io.ReadAll(resp.Body)
}

// --- roles ---

func (c *Client) ListRoles(ctx context.Context) ([]models.Role, error) {
	var out []models.Role
	return out, c.do(ctx, http.MethodGet, "/api/v1/roles", nil, &out)
}
func (c *Client) GetRole(ctx context.Context, name string) (*models.Role, error) {
	var out models.Role
	return &out, c.do(ctx, http.MethodGet, "/api/v1/roles/"+url.PathEscape(name), nil, &out)
}
func (c *Client) PutRole(ctx context.Context, r *models.Role) (*models.Role, error) {
	var out models.Role
	return &out, c.do(ctx, http.MethodPut, "/api/v1/roles/"+url.PathEscape(r.Name), r, &out)
}
func (c *Client) DeleteRole(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/roles/"+url.PathEscape(name), nil, nil)
}

// --- statuses ---

func (c *Client) ListStatuses(ctx context.Context) ([]models.Status, error) {
	var out []models.Status
	return out, c.do(ctx, http.MethodGet, "/api/v1/statuses", nil, &out)
}
func (c *Client) GetStatus(ctx context.Context, name string) (*models.Status, error) {
	var out models.Status
	return &out, c.do(ctx, http.MethodGet, "/api/v1/statuses/"+url.PathEscape(name), nil, &out)
}
func (c *Client) PutStatus(ctx context.Context, s *models.Status) (*models.Status, error) {
	var out models.Status
	return &out, c.do(ctx, http.MethodPut, "/api/v1/statuses/"+url.PathEscape(s.Name), s, &out)
}
func (c *Client) DeleteStatus(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/statuses/"+url.PathEscape(name), nil, nil)
}

// --- nodes ---

func (c *Client) ListNodes(ctx context.Context) ([]models.Node, error) {
	var out []models.Node
	return out, c.do(ctx, http.MethodGet, "/api/v1/nodes", nil, &out)
}
func (c *Client) GetNode(ctx context.Context, certname string) (*models.Node, error) {
	var out models.Node
	return &out, c.do(ctx, http.MethodGet, "/api/v1/nodes/"+url.PathEscape(certname), nil, &out)
}
func (c *Client) PutNode(ctx context.Context, n *models.Node) (*models.Node, error) {
	var out models.Node
	return &out, c.do(ctx, http.MethodPut, "/api/v1/nodes/"+url.PathEscape(n.Certname), n, &out)
}
func (c *Client) SetNodeEnvironment(ctx context.Context, certname, environment string) (*models.Node, error) {
	var out models.Node
	body := map[string]string{"environment": environment}
	return &out, c.do(ctx, http.MethodPut, "/api/v1/nodes/"+url.PathEscape(certname)+"/environment", body, &out)
}
func (c *Client) DeleteNode(ctx context.Context, certname string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/nodes/"+url.PathEscape(certname), nil, nil)
}
