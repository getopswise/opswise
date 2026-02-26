package git

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PushConfig holds git repository configuration.
type PushConfig struct {
	URL    string
	Branch string
	Token  string
}

// PushDeployment copies deployment files to the configured git repo and pushes.
func PushDeployment(cfg PushConfig, deployDir string, deploymentID int64, targetName, mode string) error {
	if cfg.URL == "" {
		return fmt.Errorf("git URL not configured")
	}

	tmpDir, err := os.MkdirTemp("", "opswise-git-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build auth URL if token provided
	cloneURL := cfg.URL
	if cfg.Token != "" {
		cloneURL, err = injectToken(cfg.URL, cfg.Token)
		if err != nil {
			return fmt.Errorf("inject token: %w", err)
		}
	}

	branch := cfg.Branch
	if branch == "" {
		branch = "main"
	}

	// Clone
	repoDir := filepath.Join(tmpDir, "repo")
	if err := runGit(tmpDir, "clone", "--branch", branch, "--depth", "1", cloneURL, repoDir); err != nil {
		// Branch might not exist, try cloning without branch
		if err := runGit(tmpDir, "clone", "--depth", "1", cloneURL, repoDir); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
	}

	// Determine source files
	var sourceDir string
	switch mode {
	case "ansible":
		sourceDir = filepath.Join(deployDir, "products", targetName, "ansible")
	case "compose":
		sourceDir = filepath.Join(deployDir, "products", targetName, "compose")
	case "helm":
		sourceDir = filepath.Join(deployDir, "products", targetName, "helm")
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}

	// Copy files into opswise-generated/<deployment-id>/
	destDir := filepath.Join(repoDir, "opswise-generated", fmt.Sprintf("%d", deploymentID))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	if err := copyDir(sourceDir, destDir); err != nil {
		return fmt.Errorf("copy files: %w", err)
	}

	// Git add, commit, push
	if err := runGit(repoDir, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	commitMsg := fmt.Sprintf("opswise: deployment #%d - %s (%s)", deploymentID, targetName, mode)
	if err := runGit(repoDir, "commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if err := runGit(repoDir, "push", "origin", branch); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func injectToken(rawURL, token string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword("oauth2", token)
	return u.String(), nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
