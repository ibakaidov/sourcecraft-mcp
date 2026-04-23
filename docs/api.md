# MCP API

## Deploy-first tools

- `run_deploy_workflow`: explicit confirm string is mandatory; by default it waits until completion.
- `wait_run`: long-polling for `created`, `prepared`, `processing` to terminal states `success`, `failed`, `canceled`, `timeout`; emits MCP progress heartbeats when `progressToken` is present.

## Generic CI tools

- `list_runs`
- `get_run`
- `run_workflow`
- `get_cube_logs`
- `list_cube_artifacts`
- `download_artifact`

## Documentation and public API tools

- `search_ci_docs`
- `list_api_operations`
- `call_api`

## Generic API call rules

- operations are discovered from the live SourceCraft OpenAPI document
- deprecated or withdrawn operations are blocked unless `allow_deprecated=true`
- `path` must match an OpenAPI path exactly, for example `/repos/{org_slug}/{repo_slug}/cicd/runs`
- `path_params` and `query` are applied after validation
- `body` sends JSON
- `raw_body_base64` plus `content_type` is available for non-JSON bodies
