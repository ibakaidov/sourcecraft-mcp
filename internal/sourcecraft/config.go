package sourcecraft

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	DefaultAPIBase  = "https://api.sourcecraft.tech"
	DefaultDocsBase = "https://sourcecraft.dev/portal/docs"
)

type Config struct {
	PAT      string
	Org      string
	Repo     string
	RepoHint string
	EnvFile  string
	APIBase  string
	DocsBase string
	WorkDir  string
}

func LoadConfig(workDir string) (Config, error) {
	cfg := Config{
		APIBase:  DefaultAPIBase,
		DocsBase: DefaultDocsBase,
		WorkDir:  workDir,
	}

	explicit := captureEnv()
	detectedOrg, detectedRepo := detectSourceCraftRemote(workDir)
	repoHint := explicit.RepoHint
	if repoHint == "" {
		repoHint = detectedRepo
	}

	values := map[string]string{}
	loadEnvInto(values, filepath.Join(userHomeDir(), ".config", "sourcecraft", "default.env"))
	if repoHint != "" {
		loadEnvInto(values, filepath.Join(userHomeDir(), ".config", "sourcecraft", repoHint+".env"))
	}
	loadEnvInto(values, filepath.Join(workDir, ".env.sourcecraft"))
	if explicit.EnvFile != "" {
		loadEnvInto(values, explicit.EnvFile)
	}

	cfg.PAT = values["SOURCECRAFT_PAT"]
	cfg.Org = values["SOURCECRAFT_ORG"]
	cfg.Repo = values["SOURCECRAFT_REPO"]
	cfg.RepoHint = values["SOURCECRAFT_REPO_HINT"]
	cfg.EnvFile = explicit.EnvFile

	if cfg.Org == "" {
		cfg.Org = detectedOrg
	}
	if cfg.Repo == "" {
		cfg.Repo = detectedRepo
	}
	if cfg.RepoHint == "" {
		cfg.RepoHint = repoHint
	}

	if explicit.PAT != "" {
		cfg.PAT = explicit.PAT
	}
	if explicit.Org != "" {
		cfg.Org = explicit.Org
	}
	if explicit.Repo != "" {
		cfg.Repo = explicit.Repo
	}
	if explicit.RepoHint != "" {
		cfg.RepoHint = explicit.RepoHint
	}

	if apiBase, ok := os.LookupEnv("SOURCECRAFT_API_BASE"); ok && apiBase != "" {
		cfg.APIBase = strings.TrimRight(apiBase, "/")
	}
	if docsBase, ok := os.LookupEnv("SOURCECRAFT_DOCS_BASE"); ok && docsBase != "" {
		cfg.DocsBase = strings.TrimRight(docsBase, "/")
	}

	if cfg.PAT == "" {
		return cfg, errors.New("missing required env: SOURCECRAFT_PAT")
	}
	return cfg, nil
}

func (c Config) ResolveRepo(org, repo string) (string, string, error) {
	if org == "" {
		org = c.Org
	}
	if repo == "" {
		repo = c.Repo
	}
	if org == "" || repo == "" {
		return "", "", errors.New("missing sourcecraft org/repo; set SOURCECRAFT_ORG and SOURCECRAFT_REPO or provide tool arguments")
	}
	return org, repo, nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

type explicitEnv struct {
	PAT      string
	Org      string
	Repo     string
	RepoHint string
	EnvFile  string
}

func captureEnv() explicitEnv {
	return explicitEnv{
		PAT:      os.Getenv("SOURCECRAFT_PAT"),
		Org:      os.Getenv("SOURCECRAFT_ORG"),
		Repo:     os.Getenv("SOURCECRAFT_REPO"),
		RepoHint: os.Getenv("SOURCECRAFT_REPO_HINT"),
		EnvFile:  os.Getenv("SOURCECRAFT_ENV_FILE"),
	}
}

func detectSourceCraftRemote(workDir string) (org string, repo string) {
	cmd := exec.Command("git", "remote", "get-url", "sourcecraft")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}
	return parseSourceCraftRemote(strings.TrimSpace(string(output)))
}

func parseSourceCraftRemote(remoteURL string) (org string, repo string) {
	prefixes := []string{
		"ssh://ssh.sourcecraft.dev/",
		"git@ssh.sourcecraft.dev:",
	}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(remoteURL, prefix) {
			continue
		}
		rest := strings.TrimPrefix(remoteURL, prefix)
		rest = strings.TrimSuffix(rest, ".git")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 {
			return "", ""
		}
		return parts[0], parts[1]
	}
	return "", ""
}

func loadEnvInto(dst map[string]string, path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseEnvValue(strings.TrimSpace(value))
		if key != "" {
			dst[key] = value
		}
	}
}

func parseEnvValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1]
		}
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func (c Config) EnvSummary() map[string]string {
	return map[string]string{
		"org":       c.Org,
		"repo":      c.Repo,
		"repo_hint": c.RepoHint,
		"env_file":  c.EnvFile,
		"api_base":  c.APIBase,
		"docs_base": c.DocsBase,
		"workdir":   c.WorkDir,
	}
}

func (c Config) String() string {
	return fmt.Sprintf("%s/%s", c.Org, c.Repo)
}
