package mcpserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aacidov/sourcecraft-mcp/internal/sourcecraft"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunDeployWorkflowRequiresConfirm(t *testing.T) {
	t.Parallel()

	svc := sourcecraft.NewService(sourcecraft.Config{
		PAT:      "token",
		Org:      "acme",
		Repo:     "demo",
		APIBase:  "https://api.example.test",
		DocsBase: "https://docs.example.test",
	}, nil)
	server := New(svc)

	_, _, err := server.runDeployWorkflow(context.Background(), nil, runDeployInput{
		Workflow: "deploy-prod",
		Target:   "service-a",
		Confirm:  "deploy deploy-prod",
	})
	if err == nil {
		t.Fatal("expected confirm validation error")
	}
}

func TestDownloadArtifactReturnsStructuredBinary(t *testing.T) {
	t.Parallel()

	var api *httptest.Server
	api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/demo/cicd/artifacts/42/wf/task/cube":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[{"local_path":"out.zip","download_url":"` + api.URL + `/download/out.zip"}]}`))
		case "/download/out.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write([]byte("PK\x03\x04"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	svc := sourcecraft.NewService(sourcecraft.Config{
		PAT:      "token",
		Org:      "acme",
		Repo:     "demo",
		APIBase:  api.URL,
		DocsBase: "https://docs.example.test",
	}, nil)
	server := New(svc)

	result, out, err := server.downloadArtifact(context.Background(), nil, downloadArtifactInput{
		cubeLocatorInput: cubeLocatorInput{
			repoArgs:     repoArgs{Org: "acme", Repo: "demo"},
			RunSlug:      "42",
			WorkflowSlug: "wf",
			TaskSlug:     "task",
			CubeSlug:     "cube",
		},
		LocalPath: "out.zip",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.BlobBase64 != base64.StdEncoding.EncodeToString([]byte("PK\x03\x04")) {
		t.Fatalf("blob mismatch: %q", out.BlobBase64)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	if _, ok := result.Content[0].(*mcp.EmbeddedResource); !ok {
		t.Fatalf("content type = %T, want *mcp.EmbeddedResource", result.Content[0])
	}
}
