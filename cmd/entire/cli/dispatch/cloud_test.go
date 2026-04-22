package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudClient_CreateDispatch_Happy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/dispatch" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["repos"] == nil {
			t.Fatalf("bad body: %v", body)
		}
		repos, ok := body["repos"].([]any)
		if !ok || len(repos) != 1 || repos[0] != "entireio/cli" {
			t.Fatalf("bad repos payload: %v", body["repos"])
		}
		if _, ok := body["repo"]; ok {
			t.Fatalf("did not expect repo in request body: %v", body)
		}
		if _, ok := body["wait"]; ok {
			t.Fatalf("did not expect wait in request body: %v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"window":{"normalized_since":"2026-04-09T00:00:00Z","normalized_until":"2026-04-16T00:00:00Z"},"covered_repos":["entireio/cli"],"repos":[],"totals":{"checkpoints":0,"used_checkpoint_count":0,"branches":0,"files_touched":0},"warnings":{"access_denied_count":0,"pending_count":0,"failed_count":0,"unknown_count":0,"uncategorized_count":0},"generated_markdown":"hi"}`)) //nolint:errcheck // test fixture response
	}))
	defer srv.Close()

	client := NewCloudClient(CloudConfig{BaseURL: srv.URL, Token: "t"})
	got, err := client.CreateDispatch(ctx, CreateDispatchRequest{
		Repos:    []string{"entireio/cli"},
		Since:    "2026-04-09T00:00:00Z",
		Until:    "2026-04-16T00:00:00Z",
		Branches: []string{"all"},
		Generate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GeneratedMarkdown != "hi" {
		t.Fatalf("bad generated markdown: %q", got.GeneratedMarkdown)
	}
}

func TestCloudClient_CreateDispatch_MultipleOrgs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/dispatch" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		orgs, ok := body["orgs"].([]any)
		if !ok || len(orgs) != 2 || orgs[0] != "entireio" || orgs[1] != "otherco" {
			t.Fatalf("bad orgs payload: %v", body["orgs"])
		}
		if _, ok := body["repos"]; ok {
			t.Fatalf("did not expect repos payload: %v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"window":{"normalized_since":"2026-04-09T00:00:00Z","normalized_until":"2026-04-16T00:00:00Z"},"covered_repos":["entireio/cli","otherco/service"],"repos":[],"generated_markdown":"hi"}`)) //nolint:errcheck // test fixture response
	}))
	defer srv.Close()

	client := NewCloudClient(CloudConfig{BaseURL: srv.URL, Token: "t"})
	got, err := client.CreateDispatch(ctx, CreateDispatchRequest{
		Orgs:     []string{"entireio", "otherco"},
		Since:    "2026-04-09T00:00:00Z",
		Until:    "2026-04-16T00:00:00Z",
		Branches: []string{},
		Generate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.GeneratedMarkdown != "hi" {
		t.Fatalf("bad generated markdown: %q", got.GeneratedMarkdown)
	}
}

func TestCloudClient_CreateDispatch_Unauthorized(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewCloudClient(CloudConfig{BaseURL: srv.URL, Token: ""})
	_, err := client.CreateDispatch(ctx, CreateDispatchRequest{Repos: []string{"x/y"}})
	if err == nil || !strings.Contains(err.Error(), "entire login") {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestNewCloudClient_DefaultHTTPClientHasNoTimeout(t *testing.T) {
	t.Parallel()

	client := NewCloudClient(CloudConfig{BaseURL: "http://example.com", Token: "t"})
	if client.http == nil {
		t.Fatal("expected http client")
	}
	if client.http.Timeout != 0 {
		t.Fatalf("expected no default http timeout, got %s", client.http.Timeout)
	}
}

func TestNewCloudClient_ConfiguredTimeoutStillApplies(t *testing.T) {
	t.Parallel()

	timeout := 45 * time.Second
	client := NewCloudClient(CloudConfig{
		BaseURL: "http://example.com",
		Token:   "t",
		Timeout: timeout,
	})
	if client.http == nil {
		t.Fatal("expected http client")
	}
	if client.http.Timeout != timeout {
		t.Fatalf("expected configured timeout %s, got %s", timeout, client.http.Timeout)
	}
}
