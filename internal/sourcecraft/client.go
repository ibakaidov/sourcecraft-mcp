package sourcecraft

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const openAPISpecURL = "https://api.sourcecraft.tech/sourcecraft.swagger.json"

var terminalStatuses = map[string]bool{
	"success":  true,
	"failed":   true,
	"canceled": true,
	"timeout":  true,
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	baseURL    string
	httpClient HTTPClient
	token      string
}

type Service struct {
	cfg     Config
	client  *Client
	docs    *DocsIndex
	openapi *OpenAPICatalog
}

type ListRunsResponse struct {
	Runs          []Run  `json:"runs"`
	NextPageToken string `json:"next_page_token,omitempty"`
}

type Run struct {
	ID            string         `json:"id,omitempty"`
	Slug          string         `json:"slug,omitempty"`
	Status        string         `json:"status,omitempty"`
	Dates         map[string]any `json:"dates,omitempty"`
	Workflows     []Workflow     `json:"workflows,omitempty"`
	EventType     string         `json:"event_type,omitempty"`
	ErrorMessages []string       `json:"error_messages,omitempty"`
	User          map[string]any `json:"user,omitempty"`
	Pull          map[string]any `json:"pull,omitempty"`
}

type Workflow struct {
	ID          string         `json:"id,omitempty"`
	Slug        string         `json:"slug,omitempty"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Dates       map[string]any `json:"dates,omitempty"`
	Tasks       []Task         `json:"tasks,omitempty"`
	Progress    map[string]any `json:"progress,omitempty"`
}

type Task struct {
	ID          string         `json:"id,omitempty"`
	Slug        string         `json:"slug,omitempty"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Dates       map[string]any `json:"dates,omitempty"`
	Cubes       []Cube         `json:"cubes,omitempty"`
	Progress    map[string]any `json:"progress,omitempty"`
	Relations   map[string]any `json:"relations,omitempty"`
}

type Cube struct {
	ID        string         `json:"id,omitempty"`
	Slug      string         `json:"slug,omitempty"`
	Status    string         `json:"status,omitempty"`
	Dates     map[string]any `json:"dates,omitempty"`
	Artifacts []Artifact     `json:"artifacts,omitempty"`
	Relations map[string]any `json:"relations,omitempty"`
}

type Artifact struct {
	ID          string         `json:"id,omitempty"`
	LocalPath   string         `json:"local_path,omitempty"`
	Status      string         `json:"status,omitempty"`
	DownloadURL string         `json:"download_url,omitempty"`
	Dates       map[string]any `json:"dates,omitempty"`
}

type GetCubeArtifactsResponse struct {
	Artifacts []Artifact `json:"artifacts"`
}

type GetCubeLogsResponse struct {
	Logs         string `json:"logs"`
	PageComplete bool   `json:"page_complete"`
	Done         bool   `json:"done"`
}

type GitRevision struct {
	Branch string `json:"branch,omitempty"`
	Tag    string `json:"tag,omitempty"`
	Commit string `json:"commit,omitempty"`
}

type InputValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type WorkflowData struct {
	Name   string       `json:"name"`
	Values []InputValue `json:"values,omitempty"`
}

type RunWorkflowsBody struct {
	Head           *GitRevision   `json:"head,omitempty"`
	ConfigRevision *GitRevision   `json:"config_revision,omitempty"`
	Workflows      []WorkflowData `json:"workflows"`
	Shared         bool           `json:"shared,omitempty"`
}

type WaitOptions struct {
	PollInterval time.Duration
	Heartbeat    time.Duration
	Timeout      time.Duration
}

type WaitResult struct {
	Run             Run           `json:"run"`
	ObservedChanges []string      `json:"observed_changes"`
	Duration        time.Duration `json:"duration"`
}

type LogStreamPage struct {
	Page         int    `json:"page"`
	Logs         string `json:"logs"`
	PageComplete bool   `json:"page_complete"`
	Done         bool   `json:"done"`
}

type DownloadedArtifact struct {
	Artifact    Artifact `json:"artifact"`
	MIMEType    string   `json:"mime_type"`
	Size        int      `json:"size"`
	Text        string   `json:"text,omitempty"`
	BlobBase64  string   `json:"blob_base64,omitempty"`
	IsText      bool     `json:"is_text"`
	DownloadURL string   `json:"download_url"`
}

type APIResult struct {
	StatusCode  int               `json:"status_code"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers,omitempty"`
	JSON        map[string]any    `json:"json,omitempty"`
	Text        string            `json:"text,omitempty"`
	BodyBase64  string            `json:"body_base64,omitempty"`
}

func NewClient(cfg Config, httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.APIBase, "/"),
		httpClient: httpClient,
		token:      cfg.PAT,
	}
}

func NewService(cfg Config, httpClient HTTPClient) *Service {
	client := NewClient(cfg, httpClient)
	return &Service{
		cfg:     cfg,
		client:  client,
		docs:    NewDocsIndex(cfg.DocsBase, nil),
		openapi: NewOpenAPICatalog(nil),
	}
}

func (s *Service) WithHTTPClient(httpClient HTTPClient) *Service {
	clone := *s
	clone.client = NewClient(s.cfg, httpClient)
	clone.docs = NewDocsIndex(s.cfg.DocsBase, httpClient)
	clone.openapi = NewOpenAPICatalog(httpClient)
	return &clone
}

func (s *Service) Config() Config {
	return s.cfg
}

func (s *Service) ListRuns(ctx context.Context, org, repo string, pageSize int, pageToken string) (ListRunsResponse, error) {
	return s.client.ListRuns(ctx, org, repo, pageSize, pageToken)
}

func (s *Service) GetRun(ctx context.Context, org, repo, runSlug string) (Run, error) {
	return s.client.GetRun(ctx, org, repo, runSlug)
}

func (s *Service) RunWorkflows(ctx context.Context, org, repo string, body RunWorkflowsBody) (Run, error) {
	return s.client.RunWorkflows(ctx, org, repo, body)
}

func (s *Service) GetCubeLogs(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug string, page int) (GetCubeLogsResponse, error) {
	return s.client.GetCubeLogs(ctx, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug, page)
}

func (s *Service) StreamCubeLogs(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug string, startPage int, poll time.Duration, maxPages int) ([]LogStreamPage, error) {
	if startPage <= 0 {
		startPage = 1
	}
	if poll <= 0 {
		poll = 2 * time.Second
	}
	pages := []LogStreamPage{}
	page := startPage
	for {
		logs, err := s.GetCubeLogs(ctx, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug, page)
		if err != nil {
			return nil, err
		}
		pages = append(pages, LogStreamPage{
			Page:         page,
			Logs:         logs.Logs,
			PageComplete: logs.PageComplete,
			Done:         logs.Done,
		})
		if logs.Done {
			return pages, nil
		}
		if logs.PageComplete {
			page++
		} else {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(poll):
			}
		}
		if maxPages > 0 && len(pages) >= maxPages {
			return pages, nil
		}
	}
}

func (s *Service) GetCubeArtifacts(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug string) ([]Artifact, error) {
	resp, err := s.client.GetCubeArtifacts(ctx, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug)
	if err != nil {
		return nil, err
	}
	return resp.Artifacts, nil
}

func (s *Service) DownloadArtifact(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug, localPath string) (DownloadedArtifact, error) {
	artifacts, err := s.GetCubeArtifacts(ctx, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug)
	if err != nil {
		return DownloadedArtifact{}, err
	}
	for _, artifact := range artifacts {
		if artifact.LocalPath != localPath {
			continue
		}
		body, contentType, err := s.client.DownloadURL(ctx, artifact.DownloadURL)
		if err != nil {
			return DownloadedArtifact{}, err
		}
		result := DownloadedArtifact{
			Artifact:    artifact,
			MIMEType:    contentType,
			Size:        len(body),
			DownloadURL: artifact.DownloadURL,
		}
		if looksLikeText(contentType, artifact.LocalPath, body) {
			result.IsText = true
			result.Text = string(body)
		} else {
			result.BlobBase64 = base64.StdEncoding.EncodeToString(body)
		}
		return result, nil
	}
	return DownloadedArtifact{}, fmt.Errorf("artifact %q not found", localPath)
}

func (s *Service) WaitRun(ctx context.Context, org, repo, runSlug string, opts WaitOptions) (WaitResult, error) {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 10 * time.Second
	}
	if opts.Heartbeat <= 0 {
		opts.Heartbeat = 30 * time.Second
	}

	start := time.Now()
	deadline := time.Time{}
	if opts.Timeout > 0 {
		deadline = start.Add(opts.Timeout)
	}

	var lastSummary string
	var changes []string
	for {
		run, err := s.GetRun(ctx, org, repo, runSlug)
		if err != nil {
			return WaitResult{}, err
		}

		summary := summarizeRun(run)
		if summary != lastSummary {
			lastSummary = summary
			changes = append(changes, summary)
		}
		if terminalStatuses[run.Status] {
			return WaitResult{
				Run:             run,
				ObservedChanges: changes,
				Duration:        time.Since(start),
			}, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return WaitResult{}, fmt.Errorf("wait timeout after %s", opts.Timeout)
		}

		select {
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
}

func summarizeRun(run Run) string {
	parts := []string{
		"run=" + run.Slug,
		"status=" + run.Status,
	}
	if len(run.Workflows) > 0 {
		items := make([]string, 0, len(run.Workflows))
		for _, wf := range run.Workflows {
			items = append(items, wf.Slug+":"+wf.Status)
		}
		parts = append(parts, "workflows="+strings.Join(items, ","))
	}
	return strings.Join(parts, " ")
}

func (c *Client) ListRuns(ctx context.Context, org, repo string, pageSize int, pageToken string) (ListRunsResponse, error) {
	query := url.Values{}
	if pageSize > 0 {
		query.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var out ListRunsResponse
	err := c.doJSON(ctx, http.MethodGet, repoCIPath(org, repo, "/cicd/runs"), query, nil, &out)
	return out, err
}

func (c *Client) GetRun(ctx context.Context, org, repo, runSlug string) (Run, error) {
	var out Run
	err := c.doJSON(ctx, http.MethodGet, repoCIPath(org, repo, "/cicd/runs/"+url.PathEscape(runSlug)), nil, nil, &out)
	return out, err
}

func (c *Client) RunWorkflows(ctx context.Context, org, repo string, body RunWorkflowsBody) (Run, error) {
	var out Run
	err := c.doJSON(ctx, http.MethodPost, repoCIPath(org, repo, "/cicd/runs"), nil, body, &out)
	return out, err
}

func (c *Client) GetCubeLogs(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug string, pageNumber int) (GetCubeLogsResponse, error) {
	query := url.Values{}
	if pageNumber > 0 {
		query.Set("page", fmt.Sprintf("%d", pageNumber))
	}
	var out GetCubeLogsResponse
	err := c.doJSON(ctx, http.MethodGet, repoCIPath(org, repo, fmt.Sprintf("/cicd/logs/%s/%s/%s/%s",
		url.PathEscape(runSlug),
		url.PathEscape(workflowSlug),
		url.PathEscape(taskSlug),
		url.PathEscape(cubeSlug),
	)), query, nil, &out)
	return out, err
}

func (c *Client) GetCubeArtifacts(ctx context.Context, org, repo, runSlug, workflowSlug, taskSlug, cubeSlug string) (GetCubeArtifactsResponse, error) {
	var out GetCubeArtifactsResponse
	err := c.doJSON(ctx, http.MethodGet, repoCIPath(org, repo, fmt.Sprintf("/cicd/artifacts/%s/%s/%s/%s",
		url.PathEscape(runSlug),
		url.PathEscape(workflowSlug),
		url.PathEscape(taskSlug),
		url.PathEscape(cubeSlug),
	)), nil, nil, &out)
	return out, err
}

func (c *Client) DownloadURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("artifact download failed: %s", resp.Status)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("sourcecraft api %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(payload)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(payload, out)
}

func repoCIPath(org, repo, suffix string) string {
	return fmt.Sprintf("/repos/%s/%s%s", url.PathEscape(org), url.PathEscape(repo), suffix)
}

func looksLikeText(contentType, name string, body []byte) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/yaml", "application/x-yaml", "application/xml":
		return true
	}
	if looksLikeTextExtension(name) {
		return true
	}
	if mediaType != "" && mediaType != "application/octet-stream" {
		return false
	}
	if len(body) == 0 {
		return true
	}
	sample := body
	if len(sample) > 512 {
		sample = sample[:512]
	}
	for _, b := range sample {
		if b == 0 {
			return false
		}
	}
	return true
}

func looksLikeTextExtension(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".log", ".txt", ".md", ".json", ".yaml", ".yml", ".xml", ".csv", ".env":
		return true
	default:
		return false
	}
}

type cacheEntry[T any] struct {
	Value     T
	FetchedAt time.Time
}

type OpenAPICatalog struct {
	httpClient HTTPClient
	mu         sync.Mutex
	cache      cacheEntry[OpenAPISpec]
	ttl        time.Duration
}

type OpenAPISpec struct {
	Paths map[string]map[string]OpenAPIOperation `json:"paths"`
}

type OpenAPIOperation struct {
	Summary     string             `json:"summary"`
	Description string             `json:"description"`
	OperationID string             `json:"operationId"`
	Deprecated  bool               `json:"deprecated"`
	Tags        []string           `json:"tags"`
	Parameters  []OpenAPIParameter `json:"parameters"`
}

type OpenAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Required    bool           `json:"required"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

type APIOperationInfo struct {
	Method      string             `json:"method"`
	Path        string             `json:"path"`
	Summary     string             `json:"summary"`
	Description string             `json:"description,omitempty"`
	OperationID string             `json:"operation_id,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Deprecated  bool               `json:"deprecated"`
	Parameters  []OpenAPIParameter `json:"parameters,omitempty"`
}

func NewOpenAPICatalog(httpClient HTTPClient) *OpenAPICatalog {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAPICatalog{
		httpClient: httpClient,
		ttl:        15 * time.Minute,
	}
}

func (o *OpenAPICatalog) Load(ctx context.Context) (OpenAPISpec, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.cache.FetchedAt.IsZero() && time.Since(o.cache.FetchedAt) < o.ttl {
		return o.cache.Value, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAPISpecURL, nil)
	if err != nil {
		return OpenAPISpec{}, err
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return OpenAPISpec{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return OpenAPISpec{}, fmt.Errorf("openapi fetch failed: %s", resp.Status)
	}
	var spec OpenAPISpec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return OpenAPISpec{}, err
	}
	o.cache = cacheEntry[OpenAPISpec]{Value: spec, FetchedAt: time.Now()}
	return spec, nil
}

func (o *OpenAPICatalog) ListOperations(ctx context.Context, tag, query string, includeDeprecated bool, limit int) ([]APIOperationInfo, error) {
	spec, err := o.Load(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	tag = strings.ToLower(strings.TrimSpace(tag))
	var result []APIOperationInfo
	for rawPath, methods := range spec.Paths {
		for method, op := range methods {
			if !includeDeprecated && (op.Deprecated || isWithdrawnOperation(op)) {
				continue
			}
			if tag != "" && !hasTag(op.Tags, tag) {
				continue
			}
			info := APIOperationInfo{
				Method:      strings.ToUpper(method),
				Path:        rawPath,
				Summary:     op.Summary,
				Description: op.Description,
				OperationID: op.OperationID,
				Tags:        op.Tags,
				Deprecated:  op.Deprecated || isWithdrawnOperation(op),
				Parameters:  normalizeParameters(op.Parameters),
			}
			if query != "" && !matchesOperation(info, query) {
				continue
			}
			result = append(result, info)
			if limit > 0 && len(result) >= limit {
				return result, nil
			}
		}
	}
	return result, nil
}

func isWithdrawnOperation(op OpenAPIOperation) bool {
	for _, tag := range op.Tags {
		if strings.EqualFold(tag, "Withdrawn") {
			return true
		}
	}
	return false
}

func hasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), wanted) {
			return true
		}
	}
	return false
}

func matchesOperation(op APIOperationInfo, query string) bool {
	blob := strings.ToLower(strings.Join([]string{
		op.Method,
		op.Path,
		op.Summary,
		op.Description,
		op.OperationID,
		strings.Join(op.Tags, " "),
	}, " "))
	return strings.Contains(blob, query)
}

func (s *Service) ListAPIOperations(ctx context.Context, tag, query string, includeDeprecated bool, limit int) ([]APIOperationInfo, error) {
	return s.openapi.ListOperations(ctx, tag, query, includeDeprecated, limit)
}

func (s *Service) OpenAPISpec(ctx context.Context) (OpenAPISpec, error) {
	return s.openapi.Load(ctx)
}

func (s *Service) CallAPI(ctx context.Context, method, rawPath string, pathParams, query map[string]string, body map[string]any, rawBodyBase64, contentType string, allowDeprecated bool) (APIResult, error) {
	spec, err := s.openapi.Load(ctx)
	if err != nil {
		return APIResult{}, err
	}
	method = strings.ToLower(strings.TrimSpace(method))
	op, ok := spec.Paths[rawPath][method]
	if !ok {
		return APIResult{}, fmt.Errorf("operation %s %s not found in SourceCraft OpenAPI", strings.ToUpper(method), rawPath)
	}
	if !allowDeprecated && (op.Deprecated || isWithdrawnOperation(op)) {
		return APIResult{}, errors.New("deprecated/withdrawn operation blocked by default; set allow_deprecated=true to call it explicitly")
	}

	resolvedPath := rawPath
	for key, value := range pathParams {
		resolvedPath = strings.ReplaceAll(resolvedPath, "{"+key+"}", url.PathEscape(value))
	}
	if strings.Contains(resolvedPath, "{") {
		return APIResult{}, fmt.Errorf("unresolved path params in %q", resolvedPath)
	}

	u, err := url.Parse(strings.TrimRight(s.cfg.APIBase, "/") + resolvedPath)
	if err != nil {
		return APIResult{}, err
	}
	q := u.Query()
	for key, value := range query {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()

	var reqBody io.Reader
	if rawBodyBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(rawBodyBase64)
		if err != nil {
			return APIResult{}, err
		}
		reqBody = bytes.NewReader(decoded)
	} else if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return APIResult{}, err
		}
		reqBody = bytes.NewReader(payload)
		if contentType == "" {
			contentType = "application/json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), u.String(), reqBody)
	if err != nil {
		return APIResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.PAT)
	if contentType != "" && reqBody != nil {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return APIResult{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return APIResult{}, err
	}

	result := APIResult{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     map[string]string{},
	}
	for key, values := range resp.Header {
		result.Headers[key] = strings.Join(values, ", ")
	}
	if looksLikeText(result.ContentType, rawPath, payload) {
		result.Text = string(payload)
		if strings.Contains(result.ContentType, "json") {
			var out map[string]any
			if err := json.Unmarshal(payload, &out); err == nil {
				result.JSON = out
			}
		}
	} else {
		result.BodyBase64 = base64.StdEncoding.EncodeToString(payload)
	}
	return result, nil
}

type DocsIndex struct {
	baseURL    string
	httpClient HTTPClient
	mu         sync.Mutex
	cache      cacheEntry[[]DocPage]
	ttl        time.Duration
}

type DocPage struct {
	Slug    string `json:"slug"`
	Lang    string `json:"lang"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary,omitempty"`
	Text    string `json:"text,omitempty"`
}

type DocSearchResult struct {
	Slug    string `json:"slug"`
	Lang    string `json:"lang"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary,omitempty"`
	Score   int    `json:"score"`
}

func NewDocsIndex(baseURL string, httpClient HTTPClient) *DocsIndex {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &DocsIndex{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		ttl:        15 * time.Minute,
	}
}

func (d *DocsIndex) Load(ctx context.Context) ([]DocPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.cache.FetchedAt.IsZero() && time.Since(d.cache.FetchedAt) < d.ttl {
		return d.cache.Value, nil
	}

	seed := []DocPage{
		{Slug: "index", Lang: "ru", URL: d.baseURL + "/ru/sourcecraft/ci-cd-ref/"},
		{Slug: "workflows", Lang: "ru", URL: d.baseURL + "/ru/sourcecraft/ci-cd-ref/workflows"},
		{Slug: "gh-actions", Lang: "ru", URL: d.baseURL + "/ru/sourcecraft/concepts/gh-actions"},
		{Slug: "self-hosted-worker", Lang: "ru", URL: d.baseURL + "/ru/sourcecraft/operations/self-hosted-worker"},
		{Slug: "api-start", Lang: "ru", URL: d.baseURL + "/ru/sourcecraft/operations/api-start"},
	}

	result := make([]DocPage, 0, len(seed))
	for _, page := range seed {
		loaded, err := d.fetchPage(ctx, page)
		if err != nil {
			return nil, err
		}
		result = append(result, loaded)
	}

	d.cache = cacheEntry[[]DocPage]{Value: result, FetchedAt: time.Now()}
	return result, nil
}

func (d *DocsIndex) fetchPage(ctx context.Context, page DocPage) (DocPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page.URL, nil)
	if err != nil {
		return DocPage{}, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return DocPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return DocPage{}, fmt.Errorf("docs fetch failed for %s: %s", page.URL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DocPage{}, err
	}
	html := string(body)
	page.Title = extractTagContent(html, "title")
	page.Summary = extractMetaDescription(html)
	page.Text = normalizeWhitespace(stripHTML(html))
	return page, nil
}

func (d *DocsIndex) Search(ctx context.Context, query, lang string, limit int) ([]DocSearchResult, error) {
	pages, err := d.Load(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	lang = strings.TrimSpace(lang)
	if limit <= 0 {
		limit = 10
	}

	results := make([]DocSearchResult, 0, limit)
	for _, page := range pages {
		if lang != "" && page.Lang != lang {
			continue
		}
		score := scoreDocument(page, query)
		if query != "" && score == 0 {
			continue
		}
		results = append(results, DocSearchResult{
			Slug:    page.Slug,
			Lang:    page.Lang,
			Title:   page.Title,
			URL:     page.URL,
			Summary: page.Summary,
			Score:   score,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (d *DocsIndex) GetPage(ctx context.Context, slug, lang string) (DocPage, error) {
	if lang == "" {
		lang = "ru"
	}
	pages, err := d.Load(ctx)
	if err != nil {
		return DocPage{}, err
	}
	for _, page := range pages {
		if page.Slug == slug && page.Lang == lang {
			return page, nil
		}
	}
	return DocPage{}, fmt.Errorf("doc page %s/%s not found", lang, slug)
}

func (s *Service) SearchDocs(ctx context.Context, query, lang string, limit int) ([]DocSearchResult, error) {
	return s.docs.Search(ctx, query, lang, limit)
}

func (s *Service) GetDocPage(ctx context.Context, slug, lang string) (DocPage, error) {
	return s.docs.GetPage(ctx, slug, lang)
}

func scoreDocument(page DocPage, query string) int {
	if query == "" {
		return 1
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 1
	}
	score := 0
	fields := []struct {
		text   string
		weight int
	}{
		{strings.ToLower(page.Slug), 6},
		{strings.ToLower(page.Title), 5},
		{strings.ToLower(page.Summary), 3},
		{strings.ToLower(page.Text), 1},
	}
	for _, term := range terms {
		for _, field := range fields {
			score += strings.Count(field.text, term) * field.weight
		}
	}
	return score
}

func normalizeParameters(in []OpenAPIParameter) []OpenAPIParameter {
	if len(in) == 0 {
		return nil
	}
	out := make([]OpenAPIParameter, 0, len(in))
	for _, parameter := range in {
		if parameter.Schema == nil {
			parameter.Schema = map[string]any{}
		}
		out = append(out, parameter)
	}
	return out
}

func stripHTML(raw string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"<br>", "\n"},
		{"<br/>", "\n"},
		{"<br />", "\n"},
		{"</p>", "\n"},
		{"</li>", "\n"},
		{"</h1>", "\n"},
		{"</h2>", "\n"},
		{"</h3>", "\n"},
	}
	for _, replacement := range replacements {
		raw = strings.ReplaceAll(raw, replacement.old, replacement.new)
	}
	lower := strings.ToLower(raw)
	for _, tag := range []string{"script", "style", "noscript"} {
		for {
			start := strings.Index(lower, "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(lower[start:], "</"+tag+">")
			if end < 0 {
				raw = raw[:start]
				lower = strings.ToLower(raw)
				break
			}
			raw = raw[:start] + raw[start+end+len(tag)+3:]
			lower = strings.ToLower(raw)
		}
	}
	var b strings.Builder
	inTag := false
	for _, r := range raw {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return htmlUnescape(b.String())
}

func normalizeWhitespace(raw string) string {
	fields := strings.Fields(raw)
	return strings.Join(fields, " ")
}

func extractTagContent(raw, tag string) string {
	lower := strings.ToLower(raw)
	open := "<" + tag
	openIdx := strings.Index(lower, open)
	if openIdx < 0 {
		return ""
	}
	openEndRel := strings.Index(lower[openIdx:], ">")
	if openEndRel < 0 {
		return ""
	}
	contentStart := openIdx + openEndRel + 1
	closeRel := strings.Index(lower[contentStart:], "</"+tag+">")
	if closeRel < 0 {
		return ""
	}
	contentEnd := contentStart + closeRel
	return strings.TrimSpace(htmlUnescape(raw[contentStart:contentEnd]))
}

func extractMetaDescription(raw string) string {
	lower := strings.ToLower(raw)
	idx := strings.Index(lower, `name="description"`)
	if idx < 0 {
		return ""
	}
	segment := raw[idx:]
	contentIdx := strings.Index(strings.ToLower(segment), `content="`)
	if contentIdx < 0 {
		return ""
	}
	contentIdx += len(`content="`)
	end := strings.Index(segment[contentIdx:], `"`)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(htmlUnescape(segment[contentIdx : contentIdx+end]))
}

func htmlUnescape(raw string) string {
	replacer := strings.NewReplacer(
		"&quot;", `"`,
		"&#39;", `'`,
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&nbsp;", " ",
	)
	return replacer.Replace(raw)
}

func defaultContentType(pathValue string) string {
	switch strings.ToLower(path.Ext(pathValue)) {
	case ".json":
		return "application/json"
	case ".txt", ".log", ".md", ".yaml", ".yml":
		return "text/plain; charset=utf-8"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
