package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yairp7/pkgfort/internal/ecosystem"
)

const defaultBaseURL = "https://api.osv.dev/v1"

// ecosystemName maps internal ecosystem names to OSV ecosystem identifiers.
// https://osv.dev/docs/#tag/vulnerability/operation/OSV_QueryAffected
var ecosystemName = map[string]string{
	"npm": "npm",
	"go":  "Go",
}

type queryPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type queryRequest struct {
	Version string       `json:"version"`
	Package queryPackage `json:"package"`
}

type queryVuln struct {
	ID string `json:"id"`
}

type queryResponse struct {
	Vulns []queryVuln `json:"vulns"`
}

type Option func(*Checker)

func WithBaseURL(url string) Option {
	return func(c *Checker) { c.baseURL = url }
}

type Checker struct {
	client  *http.Client
	baseURL string
}

func New(client *http.Client, opts ...Option) *Checker {
	c := &Checker{client: client, baseURL: defaultBaseURL}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Checker) Name() string { return "osv.dev" }

func (c *Checker) Check(ctx context.Context, pkg ecosystem.PackageInfo) ([]string, error) {
	osvEco, ok := ecosystemName[pkg.Ecosystem]
	if !ok {
		return nil, nil
	}

	body, err := json.Marshal(queryRequest{
		Version: pkg.Version,
		Package: queryPackage{Name: pkg.Package, Ecosystem: osvEco},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv.dev returned %d", resp.StatusCode)
	}

	var result queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ids := make([]string, len(result.Vulns))
	for i, v := range result.Vulns {
		ids[i] = v.ID
	}
	return ids, nil
}
