package npm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yairp7/pkgfort/internal/ecosystem"
	"github.com/yairp7/pkgfort/internal/ecosystem/npm"
)

func TestNeedsProxy(t *testing.T) {
	n := npm.NewExec()
	tests := []struct {
		args []string
		want bool
	}{
		// standard subcommands
		{[]string{"install"}, true},
		{[]string{"install", "react"}, true},
		{[]string{"i", "react"}, true},
		{[]string{"ci"}, true},
		{[]string{"update"}, true},
		// global flags before subcommand
		{[]string{"--prefix", "/some/path", "install"}, true},
		{[]string{"--prefix", "/some/path", "ci"}, true},
		{[]string{"--registry", "https://r.example.com", "install"}, true},
		// boolean flags (no value) before subcommand
		{[]string{"--global", "install"}, true},
		{[]string{"--legacy-peer-deps", "install"}, true},
		// flag=value form — prefix is not positional, subcommand is next non-flag
		{[]string{"--prefix=/some/path", "install"}, true},
		// pass-through commands
		{[]string{"run", "build"}, false},
		{[]string{"test"}, false},
		{[]string{"publish"}, false},
		{[]string{}, false},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := n.NeedsProxy(tt.args)
			if got != tt.want {
				t.Errorf("NeedsProxy(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestValidationTarget(t *testing.T) {
	n := npm.New(&http.Client{})

	tests := []struct {
		path string
		want ecosystem.ValidationTargetResult
	}{
		// Metadata without version — cannot validate, pass through
		{"/react", ecosystem.ValidationTargetResult{}},
		{"/@babel/core", ecosystem.ValidationTargetResult{}},

		// Metadata with specific version — validate
		{"/react/18.2.0", ecosystem.ValidationTargetResult{Package: "react", Version: "18.2.0", NeedsValidation: true}},
		{"/@babel/core/7.0.0", ecosystem.ValidationTargetResult{Package: "@babel/core", Version: "7.0.0", NeedsValidation: true}},

		// Tarball — validate (version always known)
		{"/react/-/react-18.2.0.tgz", ecosystem.ValidationTargetResult{Package: "react", Version: "18.2.0", NeedsValidation: true}},
		{"/@babel/core/-/core-7.0.0.tgz", ecosystem.ValidationTargetResult{Package: "@babel/core", Version: "7.0.0", NeedsValidation: true}},

		// npm internal endpoints — pass through
		{"/-/npm/v1/security/audits", ecosystem.ValidationTargetResult{}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			got := n.ValidationTarget(r)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFetchPackageInfo(t *testing.T) {
	wantTime, _ := time.Parse(time.RFC3339, "2022-06-14T20:07:51.509Z")

	t.Run("returns publish time for known version", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"time":{"18.2.0":"2022-06-14T20:07:51.509Z"}}`)
		}))
		defer srv.Close()

		n := npm.New(&http.Client{}, npm.WithUpstreamURL(srv.URL))
		got, err := n.FetchPackageInfo(context.Background(), "react", "18.2.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.PublishedAt.Equal(wantTime) {
			t.Errorf("got %v, want %v", got.PublishedAt, wantTime)
		}
	})

	t.Run("returns error on 404", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		n := npm.New(&http.Client{}, npm.WithUpstreamURL(srv.URL))
		_, err := n.FetchPackageInfo(context.Background(), "no-such-package", "1.0.0")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("returns error when version not in time map", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"time":{"1.0.0":"2020-01-01T00:00:00Z"}}`)
		}))
		defer srv.Close()

		n := npm.New(&http.Client{}, npm.WithUpstreamURL(srv.URL))
		_, err := n.FetchPackageInfo(context.Background(), "react", "99.0.0")
		if err == nil {
			t.Error("expected error for missing version, got nil")
		}
	})
}
