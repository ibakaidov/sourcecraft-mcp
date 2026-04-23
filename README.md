# sourcecraft-mcp

Public MCP server for SourceCraft CI/CD and deploy operations. It provides:

- run listing and run status inspection
- long `wait_run` polling for deployments and pipelines
- cube logs and artifacts
- curated SourceCraft CI docs search
- live SourceCraft OpenAPI catalog and generic `call_api`
- `stdio` and streamable HTTP transports from one binary

The server is deploy-first: it is optimized for operating real SourceCraft runs, waiting for long-running workflows, inspecting logs, and pulling artifacts without inventing a private CI model.

## Install

```bash
go install github.com/aacidov/sourcecraft-mcp/cmd/sourcecraft-mcp@latest
```

Build from source:

```bash
go build ./cmd/sourcecraft-mcp
```

## Configuration

Required:

- `SOURCECRAFT_PAT`

Optional:

- `SOURCECRAFT_ORG`
- `SOURCECRAFT_REPO`
- `SOURCECRAFT_REPO_HINT`
- `SOURCECRAFT_ENV_FILE`
- `SOURCECRAFT_API_BASE`
- `SOURCECRAFT_DOCS_BASE`

Resolution order matches the local `sc-ci` operator flow:

1. `~/.config/sourcecraft/default.env`
2. `~/.config/sourcecraft/<repo-hint>.env`
3. `./.env.sourcecraft`
4. `$SOURCECRAFT_ENV_FILE`
5. explicit process env overrides all files

If `SOURCECRAFT_ORG` and `SOURCECRAFT_REPO` are missing, the server tries to detect them from the git remote named `sourcecraft`.

## Run

STDIO:

```bash
sourcecraft-mcp
```

Streamable HTTP:

```bash
sourcecraft-mcp http --listen 127.0.0.1:8080 --path /mcp
```

## Codex MCP Config

Add this to `~/.codex/config.toml`:

```toml
[mcp_servers.sourcecraft]
command = "go"
args = ["run", "github.com/aacidov/sourcecraft-mcp/cmd/sourcecraft-mcp@latest"]
```

The server will read `SOURCECRAFT_PAT` plus the standard SourceCraft env files automatically.

## Main Tools

- `list_runs`
- `get_run`
- `run_workflow`
- `run_deploy_workflow`
- `wait_run`
- `get_cube_logs`
- `list_cube_artifacts`
- `download_artifact`
- `search_ci_docs`
- `list_api_operations`
- `call_api`
- `env`

## Resources

- `sourcecraft://docs/ci/index`
- `sourcecraft://docs/ci/{lang}/{slug}`
- `sourcecraft://openapi/sourcecraft.swagger.json`

## Documentation

- [docs/ci-authoring.md](docs/ci-authoring.md)
- [docs/api.md](docs/api.md)

## Examples

Run a deploy workflow:

```json
{
  "workflow": "deploy-prod",
  "target": "service-b",
  "confirm": "deploy deploy-prod service-b",
  "inputs": {
    "DEPLOY_TAG": "sha-5922b01"
  }
}
```

Wait for a run:

```json
{
  "run_slug": "46",
  "poll_seconds": 10,
  "heartbeat_seconds": 30,
  "timeout_seconds": 3600
}
```

The server emits MCP progress notifications while waiting when the client provides a `progressToken`, including heartbeat updates if run state does not change.

## Release Notes

- `v0.1.1`: fixed long-running `wait_run`/`run_*_workflow(wait=true)` calls by wiring `heartbeat_seconds` to progress notifications so MCP clients can keep long calls alive.

## Validation

Verified against a real SourceCraft repository:

- environment autodetection from `sourcecraft` git remote
- live run inspection
- `wait_run` on an existing completed run
- log retrieval
- artifact listing and text artifact download
- live OpenAPI discovery and generic API calls

## License

MIT
