package sourcecraft

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseSourceCraftRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		remote string
		org    string
		repo   string
	}{
		{"ssh://ssh.sourcecraft.dev/acme/demo.git", "acme", "demo"},
		{"git@ssh.sourcecraft.dev:acme/demo.git", "acme", "demo"},
	}

	for _, tt := range tests {
		org, repo := parseSourceCraftRemote(tt.remote)
		if org != tt.org || repo != tt.repo {
			t.Fatalf("parseSourceCraftRemote(%q) = %q/%q, want %q/%q", tt.remote, org, repo, tt.org, tt.repo)
		}
	}
}

func TestLoadConfigResolutionOrder(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(home, ".config", "sourcecraft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(home, ".config", "sourcecraft", "default.env"), "SOURCECRAFT_PAT=default\n")
	writeFile(t, filepath.Join(home, ".config", "sourcecraft", "demo.env"), "SOURCECRAFT_PAT=repofile\nSOURCECRAFT_ORG=file-org\n")
	writeFile(t, filepath.Join(repo, ".env.sourcecraft"), "SOURCECRAFT_REPO=local-repo\n")
	extra := filepath.Join(tmp, "extra.env")
	writeFile(t, extra, "SOURCECRAFT_REPO_HINT=extra-hint\n")

	t.Setenv("HOME", home)
	t.Setenv("SOURCECRAFT_ENV_FILE", extra)
	t.Setenv("SOURCECRAFT_PAT", "explicit")

	git := exec.Command("git", "init")
	git.Dir = repo
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, out)
	}
	remote := exec.Command("git", "remote", "add", "sourcecraft", "git@ssh.sourcecraft.dev:detected/demo.git")
	remote.Dir = repo
	if out, err := remote.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v: %s", err, out)
	}

	cfg, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PAT != "explicit" {
		t.Fatalf("PAT = %q, want explicit", cfg.PAT)
	}
	if cfg.Org != "file-org" {
		t.Fatalf("Org = %q, want file-org", cfg.Org)
	}
	if cfg.Repo != "local-repo" {
		t.Fatalf("Repo = %q, want local-repo", cfg.Repo)
	}
	if cfg.RepoHint != "extra-hint" {
		t.Fatalf("RepoHint = %q, want extra-hint", cfg.RepoHint)
	}
}

func TestLoadConfigWithoutPATStillSucceeds(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(home, ".config", "sourcecraft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(repo, ".env.sourcecraft"), "SOURCECRAFT_ORG=acme\nSOURCECRAFT_REPO=demo\n")

	t.Setenv("HOME", home)
	t.Setenv("SOURCECRAFT_PAT", "")

	cfg, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HasPAT() {
		t.Fatal("expected config without PAT to remain unauthenticated")
	}
	if got := cfg.EnvSummary()["auth_configured"]; got != "false" {
		t.Fatalf("auth_configured = %q, want false", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
