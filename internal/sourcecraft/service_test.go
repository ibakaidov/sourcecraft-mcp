package sourcecraft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWaitRunTracksTransitions(t *testing.T) {
	t.Parallel()

	var calls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/demo/cicd/runs/42" {
			http.NotFound(w, r)
			return
		}
		statuses := []string{"created", "prepared", "processing", "success"}
		if calls >= len(statuses) {
			calls = len(statuses) - 1
		}
		_ = json.NewEncoder(w).Encode(Run{Slug: "42", Status: statuses[calls]})
		calls++
	}))
	defer api.Close()

	svc := NewService(Config{PAT: "token", APIBase: api.URL, DocsBase: "https://example.com"}, nil)
	result, err := svc.WaitRun(context.Background(), "acme", "demo", "42", WaitOptions{
		PollInterval: 5 * time.Millisecond,
		Heartbeat:    5 * time.Millisecond,
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != "success" {
		t.Fatalf("final status = %q, want success", result.Run.Status)
	}
	if len(result.ObservedChanges) < 3 {
		t.Fatalf("observed changes = %v, want transitions", result.ObservedChanges)
	}
}

func TestStreamCubeLogsFollowsPages(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RawQuery {
		case "page=1":
			_ = json.NewEncoder(w).Encode(GetCubeLogsResponse{Logs: "one\n", PageComplete: true, Done: false})
		case "page=2":
			_ = json.NewEncoder(w).Encode(GetCubeLogsResponse{Logs: "two\n", PageComplete: false, Done: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	svc := NewService(Config{PAT: "token", APIBase: api.URL, DocsBase: "https://example.com"}, nil)
	pages, err := svc.StreamCubeLogs(context.Background(), "acme", "demo", "42", "wf", "task", "cube", 1, 5*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}
}

func TestOpenAPICatalogFiltering(t *testing.T) {
	t.Parallel()

	openapi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(OpenAPISpec{
			Paths: map[string]map[string]OpenAPIOperation{
				"/repos/{org_slug}/{repo_slug}/cicd/runs": {
					"get": {Summary: "List CI Runs", Tags: []string{"CI/CD"}, Parameters: []OpenAPIParameter{{Name: "body", In: "body", Schema: nil}}},
				},
				"/old": {
					"get": {Summary: "Old", Tags: []string{"Withdrawn"}},
				},
			},
		})
	}))
	defer openapi.Close()

	catalog := NewOpenAPICatalog(openapi.Client())
	oldURL := openAPISpecURL
	defer func() {
		_ = oldURL
	}()

	// Use the test server by shadowing the client and path through a round-tripper expectation.
	catalog.httpClient = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = openapi.Listener.Addr().String()
		return openapi.Client().Do(req)
	})

	ops, err := catalog.ListOperations(context.Background(), "ci", "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("len(ops) = %d, want 1", len(ops))
	}
	if ops[0].Parameters[0].Schema == nil {
		t.Fatal("expected nil schema to be normalized")
	}
}

func TestScoreDocumentSplitsQueryTerms(t *testing.T) {
	t.Parallel()

	page := DocPage{
		Slug:    "workflows",
		Title:   "SourceCraft workflows",
		Summary: "Workflow inputs and cubes",
		Text:    "tasks cubes inputs",
	}
	score := scoreDocument(page, "workflows inputs cubes")
	if score == 0 {
		t.Fatal("expected multi-term search query to produce a score")
	}
}

func TestLooksLikeTextTreatsOctetStreamLogsAsText(t *testing.T) {
	t.Parallel()

	if !looksLikeText("application/octet-stream", ".debug/publish-images.log", []byte("line 1\nline 2\n")) {
		t.Fatal("expected .log artifact with octet-stream content type to be treated as text")
	}
}

func TestExtractTagContentReadsInnerTitle(t *testing.T) {
	t.Parallel()

	html := `<!doctype html><html><head><title>SourceCraft Workflows</title></head><body></body></html>`
	if got := extractTagContent(html, "title"); got != "SourceCraft Workflows" {
		t.Fatalf("extractTagContent(...) = %q, want %q", got, "SourceCraft Workflows")
	}
}

func TestListRunsRequiresPAT(t *testing.T) {
	t.Parallel()

	svc := NewService(Config{APIBase: "https://example.com", DocsBase: "https://example.com"}, nil)
	_, err := svc.ListRuns(context.Background(), "acme", "demo", 10, "")
	if err == nil {
		t.Fatal("expected missing PAT error")
	}
	if !strings.Contains(err.Error(), "SOURCECRAFT_PAT") {
		t.Fatalf("error = %q, want SOURCECRAFT_PAT hint", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}
