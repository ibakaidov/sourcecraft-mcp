# SourceCraft CI Authoring

The MCP server does not invent a private CI format. It follows the official SourceCraft model:

- repository config file: `.sourcecraft/ci.yaml`
- workflows are started by name
- workflow inputs are `NAME=VALUE`
- SourceCraft CI entities are `run -> workflow -> task -> cube`
- logs and artifacts are addressed by `run/workflow/task/cube`

The server exposes two docs-oriented surfaces:

- `search_ci_docs` for keyword search over a curated SourceCraft CI/API corpus
- `sourcecraft://docs/ci/{lang}/{slug}` resources for direct reads from the same corpus

Useful SourceCraft pages:

- `index`: CI/CD reference
- `workflows`: workflow syntax and inputs
- `gh-actions`: GitHub Actions integration notes
- `self-hosted-worker`: worker operations
- `api-start`: REST API overview
