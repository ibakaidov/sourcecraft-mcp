package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aacidov/sourcecraft-mcp/internal/sourcecraft"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "v0.1.1"

type Server struct {
	*mcp.Server
	service *sourcecraft.Service
}

func NewFromEnv(workDir string) (*mcp.Server, error) {
	cfg, err := sourcecraft.LoadConfig(workDir)
	if err != nil {
		return nil, err
	}
	return New(sourcecraft.NewService(cfg, nil)).Server, nil
}

func New(service *sourcecraft.Service) *Server {
	server := &Server{
		Server:  mcp.NewServer(&mcp.Implementation{Name: "sourcecraft-mcp", Version: version}, nil),
		service: service,
	}
	server.registerTools()
	server.registerResources()
	return server
}

func (s *Server) registerTools() {
	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "list_runs",
		Description: "List SourceCraft CI/CD runs in a repository.",
	}, s.listRuns)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "get_run",
		Description: "Get the current state of a SourceCraft CI/CD run.",
	}, s.getRun)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "run_workflow",
		Description: "Run a SourceCraft workflow and optionally wait for completion.",
	}, s.runWorkflow)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "run_deploy_workflow",
		Description: "Run a deploy workflow with explicit confirmation and wait for completion by default.",
	}, s.runDeployWorkflow)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "wait_run",
		Description: "Wait for a SourceCraft run to reach a terminal status.",
	}, s.waitRun)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "get_cube_logs",
		Description: "Read SourceCraft cube logs for one page or stream pages until completion.",
	}, s.getCubeLogs)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "list_cube_artifacts",
		Description: "List artifacts for a SourceCraft cube.",
	}, s.listCubeArtifacts)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "download_artifact",
		Description: "Download a specific SourceCraft artifact by local_path.",
	}, s.downloadArtifact)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "search_ci_docs",
		Description: "Search curated SourceCraft CI/CD and API documentation pages.",
	}, s.searchCIDocs)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "list_api_operations",
		Description: "List SourceCraft public API operations from the live OpenAPI catalog.",
	}, s.listAPIOperations)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "call_api",
		Description: "Call a SourceCraft public API operation by path and method using the live OpenAPI catalog.",
	}, s.callAPI)

	mcp.AddTool(s.Server, &mcp.Tool{
		Name:        "env",
		Description: "Show the resolved SourceCraft environment for this MCP server.",
	}, s.env)
}

func (s *Server) registerResources() {
	handler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := req.Params.URI
		switch {
		case uri == "sourcecraft://docs/ci/index":
			pages, err := s.service.SearchDocs(ctx, "", "ru", 100)
			if err != nil {
				return nil, err
			}
			payload, err := json.MarshalIndent(pages, "", "  ")
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      uri,
					MIMEType: "application/json",
					Text:     string(payload),
				}},
			}, nil
		case uri == "sourcecraft://openapi/sourcecraft.swagger.json":
			spec, err := s.service.OpenAPISpec(ctx)
			if err != nil {
				return nil, err
			}
			payload, err := json.MarshalIndent(spec, "", "  ")
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      uri,
					MIMEType: "application/json",
					Text:     string(payload),
				}},
			}, nil
		case strings.HasPrefix(uri, "sourcecraft://docs/ci/"):
			docURI, err := url.Parse(uri)
			if err != nil {
				return nil, err
			}
			parts := strings.Split(strings.TrimPrefix(docURI.Path, "/"), "/")
			if docURI.Host != "docs" || len(parts) != 3 || parts[0] != "ci" {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			lang, slug := parts[1], parts[2]
			page, err := s.service.GetDocPage(ctx, slug, lang)
			if err != nil {
				return nil, mcp.ResourceNotFoundError(uri)
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      uri,
					MIMEType: "text/plain; charset=utf-8",
					Text:     page.Title + "\n\n" + page.Summary + "\n\n" + page.URL + "\n\n" + page.Text,
				}},
			}, nil
		default:
			return nil, mcp.ResourceNotFoundError(uri)
		}
	}

	s.Server.AddResource(&mcp.Resource{
		URI:         "sourcecraft://docs/ci/index",
		Name:        "SourceCraft CI Docs Index",
		Description: "Curated SourceCraft CI/CD documentation index.",
		MIMEType:    "application/json",
	}, handler)

	s.Server.AddResource(&mcp.Resource{
		URI:         "sourcecraft://openapi/sourcecraft.swagger.json",
		Name:        "SourceCraft OpenAPI",
		Description: "Live SourceCraft OpenAPI specification.",
		MIMEType:    "application/json",
	}, handler)

	s.Server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "sourcecraft://docs/ci/{lang}/{slug}",
		Name:        "SourceCraft CI Doc Page",
		Description: "Read a specific curated SourceCraft documentation page.",
		MIMEType:    "text/plain",
	}, handler)
}

type repoArgs struct {
	Org  string `json:"org,omitempty" jsonschema:"SourceCraft org slug; defaults to resolved environment"`
	Repo string `json:"repo,omitempty" jsonschema:"SourceCraft repo slug; defaults to resolved environment"`
}

type listRunsInput struct {
	repoArgs
	PageSize  int    `json:"page_size,omitempty" jsonschema:"Maximum number of runs to return"`
	PageToken string `json:"page_token,omitempty" jsonschema:"Pagination token from a previous list_runs response"`
}

type getRunInput struct {
	repoArgs
	RunSlug string `json:"run_slug" jsonschema:"Run slug to inspect"`
}

type revisionInput struct {
	Branch string `json:"branch,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Commit string `json:"commit,omitempty"`
}

type runWorkflowInput struct {
	repoArgs
	Workflow         string            `json:"workflow" jsonschema:"Workflow name from .sourcecraft/ci.yaml"`
	Inputs           map[string]string `json:"inputs,omitempty" jsonschema:"Workflow NAME=VALUE inputs"`
	Head             revisionInput     `json:"head,omitempty"`
	ConfigRevision   revisionInput     `json:"config_revision,omitempty"`
	Shared           bool              `json:"shared,omitempty"`
	Wait             bool              `json:"wait,omitempty"`
	PollSeconds      int               `json:"poll_seconds,omitempty"`
	HeartbeatSeconds int               `json:"heartbeat_seconds,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
}

type runWorkflowOutput struct {
	Run    sourcecraft.Run         `json:"run"`
	Waited bool                    `json:"waited"`
	Final  *sourcecraft.WaitResult `json:"final,omitempty"`
	Env    map[string]string       `json:"env,omitempty"`
}

type runDeployInput struct {
	repoArgs
	Workflow         string            `json:"workflow" jsonschema:"Deploy workflow name"`
	Target           string            `json:"target,omitempty" jsonschema:"Deploy target, if your workflow uses it"`
	Inputs           map[string]string `json:"inputs,omitempty"`
	Confirm          string            `json:"confirm" jsonschema:"Explicit confirmation string"`
	Head             revisionInput     `json:"head,omitempty"`
	ConfigRevision   revisionInput     `json:"config_revision,omitempty"`
	Shared           bool              `json:"shared,omitempty"`
	Wait             *bool             `json:"wait,omitempty"`
	PollSeconds      int               `json:"poll_seconds,omitempty"`
	HeartbeatSeconds int               `json:"heartbeat_seconds,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
}

type waitRunInput struct {
	repoArgs
	RunSlug          string `json:"run_slug"`
	PollSeconds      int    `json:"poll_seconds,omitempty"`
	HeartbeatSeconds int    `json:"heartbeat_seconds,omitempty"`
	TimeoutSeconds   int    `json:"timeout_seconds,omitempty"`
}

type getCubeLogsInput struct {
	repoArgs
	RunSlug      string `json:"run_slug"`
	WorkflowSlug string `json:"workflow_slug"`
	TaskSlug     string `json:"task_slug"`
	CubeSlug     string `json:"cube_slug"`
	Page         int    `json:"page,omitempty"`
	Follow       bool   `json:"follow,omitempty"`
	PollSeconds  int    `json:"poll_seconds,omitempty"`
	MaxPages     int    `json:"max_pages,omitempty"`
}

type getCubeLogsOutput struct {
	Pages []sourcecraft.LogStreamPage `json:"pages"`
}

type listCubeArtifactsOutput struct {
	Artifacts []sourcecraft.Artifact `json:"artifacts"`
}

type cubeLocatorInput struct {
	repoArgs
	RunSlug      string `json:"run_slug"`
	WorkflowSlug string `json:"workflow_slug"`
	TaskSlug     string `json:"task_slug"`
	CubeSlug     string `json:"cube_slug"`
}

type downloadArtifactInput struct {
	cubeLocatorInput
	LocalPath string `json:"local_path"`
}

type searchDocsInput struct {
	Query string `json:"query,omitempty"`
	Lang  string `json:"lang,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type searchDocsOutput struct {
	Results []sourcecraft.DocSearchResult `json:"results"`
}

type listAPIOperationsInput struct {
	Tag               string `json:"tag,omitempty"`
	Query             string `json:"query,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	IncludeDeprecated bool   `json:"include_deprecated,omitempty"`
}

type listAPIOperationsOutput struct {
	Operations []sourcecraft.APIOperationInfo `json:"operations"`
}

type callAPIInput struct {
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	PathParams      map[string]string `json:"path_params,omitempty"`
	Query           map[string]string `json:"query,omitempty"`
	Body            map[string]any    `json:"body,omitempty"`
	RawBodyBase64   string            `json:"raw_body_base64,omitempty"`
	ContentType     string            `json:"content_type,omitempty"`
	AllowDeprecated bool              `json:"allow_deprecated,omitempty"`
}

func (s *Server) listRuns(ctx context.Context, _ *mcp.CallToolRequest, in listRunsInput) (*mcp.CallToolResult, sourcecraft.ListRunsResponse, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, sourcecraft.ListRunsResponse{}, err
	}
	out, err := s.service.ListRuns(ctx, org, repo, in.PageSize, in.PageToken)
	return nil, out, err
}

func (s *Server) getRun(ctx context.Context, _ *mcp.CallToolRequest, in getRunInput) (*mcp.CallToolResult, sourcecraft.Run, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, sourcecraft.Run{}, err
	}
	out, err := s.service.GetRun(ctx, org, repo, in.RunSlug)
	return nil, out, err
}

func (s *Server) runWorkflow(ctx context.Context, req *mcp.CallToolRequest, in runWorkflowInput) (*mcp.CallToolResult, runWorkflowOutput, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}

	body, err := buildRunBody(in.Workflow, in.Inputs, in.Head, in.ConfigRevision, in.Shared)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}
	run, err := s.service.RunWorkflows(ctx, org, repo, body)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}
	out := runWorkflowOutput{Run: run, Waited: in.Wait}
	if in.Wait {
		waitOptions := toWaitOptions(in.PollSeconds, in.HeartbeatSeconds, in.TimeoutSeconds)
		waitOptions.OnProgress = waitRunProgressNotifier(ctx, req)
		waitResult, err := s.service.WaitRun(ctx, org, repo, run.Slug, waitOptions)
		if err != nil {
			return nil, runWorkflowOutput{}, err
		}
		out.Final = &waitResult
	}
	return nil, out, nil
}

func (s *Server) runDeployWorkflow(ctx context.Context, req *mcp.CallToolRequest, in runDeployInput) (*mcp.CallToolResult, runWorkflowOutput, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}
	expectedConfirm := "deploy " + in.Workflow
	if in.Target != "" {
		expectedConfirm += " " + in.Target
	}
	if strings.TrimSpace(in.Confirm) != expectedConfirm {
		return nil, runWorkflowOutput{}, fmt.Errorf("invalid confirm string; expected %q", expectedConfirm)
	}

	inputs := copyStringMap(in.Inputs)
	if in.Target != "" {
		if _, ok := inputs["TARGET"]; !ok {
			inputs["TARGET"] = in.Target
		}
	}
	body, err := buildRunBody(in.Workflow, inputs, in.Head, in.ConfigRevision, in.Shared)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}
	run, err := s.service.RunWorkflows(ctx, org, repo, body)
	if err != nil {
		return nil, runWorkflowOutput{}, err
	}

	wait := true
	if in.Wait != nil {
		wait = *in.Wait
	}
	out := runWorkflowOutput{Run: run, Waited: wait}
	if wait {
		waitOptions := toWaitOptions(in.PollSeconds, in.HeartbeatSeconds, in.TimeoutSeconds)
		waitOptions.OnProgress = waitRunProgressNotifier(ctx, req)
		waitResult, err := s.service.WaitRun(ctx, org, repo, run.Slug, waitOptions)
		if err != nil {
			return nil, runWorkflowOutput{}, err
		}
		out.Final = &waitResult
	}
	return nil, out, nil
}

func (s *Server) waitRun(ctx context.Context, req *mcp.CallToolRequest, in waitRunInput) (*mcp.CallToolResult, sourcecraft.WaitResult, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, sourcecraft.WaitResult{}, err
	}
	waitOptions := toWaitOptions(in.PollSeconds, in.HeartbeatSeconds, in.TimeoutSeconds)
	waitOptions.OnProgress = waitRunProgressNotifier(ctx, req)
	out, err := s.service.WaitRun(ctx, org, repo, in.RunSlug, waitOptions)
	return nil, out, err
}

func (s *Server) getCubeLogs(ctx context.Context, _ *mcp.CallToolRequest, in getCubeLogsInput) (*mcp.CallToolResult, getCubeLogsOutput, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, getCubeLogsOutput{}, err
	}
	if !in.Follow {
		page := in.Page
		if page == 0 {
			page = 1
		}
		logs, err := s.service.GetCubeLogs(ctx, org, repo, in.RunSlug, in.WorkflowSlug, in.TaskSlug, in.CubeSlug, page)
		if err != nil {
			return nil, getCubeLogsOutput{}, err
		}
		return nil, getCubeLogsOutput{
			Pages: []sourcecraft.LogStreamPage{{
				Page:         page,
				Logs:         logs.Logs,
				PageComplete: logs.PageComplete,
				Done:         logs.Done,
			}},
		}, nil
	}

	pages, err := s.service.StreamCubeLogs(ctx, org, repo, in.RunSlug, in.WorkflowSlug, in.TaskSlug, in.CubeSlug, in.Page, durationSeconds(in.PollSeconds, 2*time.Second), in.MaxPages)
	if err != nil {
		return nil, getCubeLogsOutput{}, err
	}
	return nil, getCubeLogsOutput{Pages: pages}, nil
}

func (s *Server) listCubeArtifacts(ctx context.Context, _ *mcp.CallToolRequest, in cubeLocatorInput) (*mcp.CallToolResult, listCubeArtifactsOutput, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, listCubeArtifactsOutput{}, err
	}
	out, err := s.service.GetCubeArtifacts(ctx, org, repo, in.RunSlug, in.WorkflowSlug, in.TaskSlug, in.CubeSlug)
	return nil, listCubeArtifactsOutput{Artifacts: out}, err
}

func (s *Server) downloadArtifact(ctx context.Context, _ *mcp.CallToolRequest, in downloadArtifactInput) (*mcp.CallToolResult, sourcecraft.DownloadedArtifact, error) {
	org, repo, err := s.resolveRepo(in.Org, in.Repo)
	if err != nil {
		return nil, sourcecraft.DownloadedArtifact{}, err
	}
	out, err := s.service.DownloadArtifact(ctx, org, repo, in.RunSlug, in.WorkflowSlug, in.TaskSlug, in.CubeSlug, in.LocalPath)
	if err != nil {
		return nil, sourcecraft.DownloadedArtifact{}, err
	}

	result := &mcp.CallToolResult{}
	if out.IsText {
		result.Content = []mcp.Content{&mcp.TextContent{Text: out.Text}}
	} else if out.BlobBase64 != "" {
		blob, err := decodeBase64(out.BlobBase64)
		if err != nil {
			return nil, sourcecraft.DownloadedArtifact{}, err
		}
		result.Content = []mcp.Content{&mcp.EmbeddedResource{
			Resource: &mcp.ResourceContents{
				URI:      "sourcecraft://artifact/" + out.Artifact.LocalPath,
				MIMEType: nonEmpty(out.MIMEType, sourcecraftDefaultContentType(out.Artifact.LocalPath)),
				Blob:     blob,
			},
		}}
	}
	return result, out, nil
}

func (s *Server) searchCIDocs(ctx context.Context, _ *mcp.CallToolRequest, in searchDocsInput) (*mcp.CallToolResult, searchDocsOutput, error) {
	out, err := s.service.SearchDocs(ctx, in.Query, in.Lang, in.Limit)
	return nil, searchDocsOutput{Results: out}, err
}

func (s *Server) listAPIOperations(ctx context.Context, _ *mcp.CallToolRequest, in listAPIOperationsInput) (*mcp.CallToolResult, listAPIOperationsOutput, error) {
	out, err := s.service.ListAPIOperations(ctx, in.Tag, in.Query, in.IncludeDeprecated, in.Limit)
	return nil, listAPIOperationsOutput{Operations: out}, err
}

func (s *Server) callAPI(ctx context.Context, _ *mcp.CallToolRequest, in callAPIInput) (*mcp.CallToolResult, sourcecraft.APIResult, error) {
	out, err := s.service.CallAPI(ctx, in.Method, in.Path, in.PathParams, in.Query, in.Body, in.RawBodyBase64, in.ContentType, in.AllowDeprecated)
	return nil, out, err
}

func (s *Server) env(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]string, error) {
	return nil, s.service.Config().EnvSummary(), nil
}

func (s *Server) resolveRepo(org, repo string) (string, string, error) {
	return s.service.Config().ResolveRepo(org, repo)
}

func buildRunBody(workflow string, inputs map[string]string, head revisionInput, configRevision revisionInput, shared bool) (sourcecraft.RunWorkflowsBody, error) {
	if strings.TrimSpace(workflow) == "" {
		return sourcecraft.RunWorkflowsBody{}, errors.New("workflow is required")
	}
	body := sourcecraft.RunWorkflowsBody{
		Workflows: []sourcecraft.WorkflowData{{
			Name:   workflow,
			Values: flattenInputs(inputs),
		}},
		Shared: shared,
	}
	if hasRevision(head) {
		revision, err := convertRevision(head)
		if err != nil {
			return sourcecraft.RunWorkflowsBody{}, err
		}
		body.Head = &revision
	}
	if hasRevision(configRevision) {
		revision, err := convertRevision(configRevision)
		if err != nil {
			return sourcecraft.RunWorkflowsBody{}, err
		}
		body.ConfigRevision = &revision
	}
	return body, nil
}

func flattenInputs(inputs map[string]string) []sourcecraft.InputValue {
	if len(inputs) == 0 {
		return nil
	}
	values := make([]sourcecraft.InputValue, 0, len(inputs))
	for key, value := range inputs {
		values = append(values, sourcecraft.InputValue{Name: key, Value: value})
	}
	return values
}

func hasRevision(in revisionInput) bool {
	return strings.TrimSpace(in.Branch) != "" || strings.TrimSpace(in.Tag) != "" || strings.TrimSpace(in.Commit) != ""
}

func convertRevision(in revisionInput) (sourcecraft.GitRevision, error) {
	revision := sourcecraft.GitRevision{
		Branch: strings.TrimSpace(in.Branch),
		Tag:    strings.TrimSpace(in.Tag),
		Commit: strings.TrimSpace(in.Commit),
	}
	count := 0
	if revision.Branch != "" {
		count++
	}
	if revision.Tag != "" {
		count++
	}
	if revision.Commit != "" {
		count++
	}
	if count != 1 {
		return sourcecraft.GitRevision{}, errors.New("exactly one of branch, tag, commit must be provided in a git revision")
	}
	return revision, nil
}

func toWaitOptions(pollSeconds, heartbeatSeconds, timeoutSeconds int) sourcecraft.WaitOptions {
	return sourcecraft.WaitOptions{
		PollInterval: durationSeconds(pollSeconds, 10*time.Second),
		Heartbeat:    durationSeconds(heartbeatSeconds, 30*time.Second),
		Timeout:      durationSeconds(timeoutSeconds, 0),
	}
}

func durationSeconds(value int, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func decodeBase64(in string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(in)
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func sourcecraftDefaultContentType(localPath string) string {
	return "application/octet-stream"
}

func waitRunProgressNotifier(ctx context.Context, req *mcp.CallToolRequest) func(sourcecraft.WaitProgressUpdate) error {
	if req == nil || req.Session == nil {
		return nil
	}
	if req.Params == nil {
		return nil
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return nil
	}
	return func(update sourcecraft.WaitProgressUpdate) error {
		message := update.Summary
		if !update.Changed {
			runSlug := strings.TrimSpace(update.Run.Slug)
			if runSlug == "" {
				runSlug = "unknown"
			}
			status := strings.TrimSpace(update.Run.Status)
			if status == "" {
				status = "processing"
			}
			message = fmt.Sprintf("run=%s status=%s waiting elapsed=%s", runSlug, status, update.Elapsed.Round(time.Second))
		}
		if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Message:       message,
			Progress:      update.Elapsed.Seconds(),
		}); err != nil {
			return nil
		}
		return nil
	}
}
