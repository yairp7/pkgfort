package gomod_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yairp7/pkgfort/internal/ecosystem"
	"github.com/yairp7/pkgfort/internal/ecosystem/gomod"
)

func TestValidationTarget(t *testing.T) {
	g := gomod.New(&http.Client{})

	tests := []struct {
		path string
		want ecosystem.ValidationTargetResult
	}{
		// validated endpoints
		{"/github.com/foo/bar/@v/v1.0.0.info", ecosystem.ValidationTargetResult{Package: "github.com/foo/bar", Version: "v1.0.0", NeedsValidation: true}},
		{"/github.com/foo/bar/@v/v1.0.0.mod", ecosystem.ValidationTargetResult{Package: "github.com/foo/bar", Version: "v1.0.0", NeedsValidation: true}},
		{"/github.com/foo/bar/@v/v1.0.0.zip", ecosystem.ValidationTargetResult{Package: "github.com/foo/bar", Version: "v1.0.0", NeedsValidation: true}},

		// pass-through endpoints
		{"/github.com/foo/bar/@v/list", ecosystem.ValidationTargetResult{}},
		{"/github.com/foo/bar/@latest", ecosystem.ValidationTargetResult{}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			got := g.ValidationTarget(r)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFetchPackageInfo(t *testing.T) {
	wantTime, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")

	t.Run("returns publish time from .info response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`)
		}))
		defer srv.Close()

		g := gomod.New(&http.Client{}, gomod.WithUpstreamURLs([]string{srv.URL}))
		got, err := g.FetchPackageInfo(context.Background(), "github.com/foo/bar", "v1.0.0")
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

		g := gomod.New(&http.Client{}, gomod.WithUpstreamURLs([]string{srv.URL}))
		_, err := g.FetchPackageInfo(context.Background(), "github.com/no/such", "v1.0.0")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}
