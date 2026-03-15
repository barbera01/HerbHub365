package gitpublish

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"HerbHub365/services/blog-poster/internal/config"
)

type Publisher struct {
	config config.GitConfig
}

func NewPublisher(cfg config.GitConfig) *Publisher {
	return &Publisher{config: cfg}
}

func (p *Publisher) PublishPost(ctx context.Context, postPath string, day time.Time) error {
	if !p.config.PublishEnabled {
		return nil
	}

	repoDir, err := filepath.Abs(p.config.RepoDir)
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}

	absolutePostPath, err := filepath.Abs(postPath)
	if err != nil {
		return fmt.Errorf("resolve post path: %w", err)
	}

	relPath, err := filepath.Rel(repoDir, absolutePostPath)
	if err != nil {
		return fmt.Errorf("resolve repo-relative path: %w", err)
	}
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("generated post is outside git repo: %s", absolutePostPath)
	}

	if err := p.run(ctx, nil, "add generated post", "git", "-C", repoDir, "add", "--", relPath); err != nil {
		return err
	}

	hasChanges, err := p.hasStagedChanges(ctx, repoDir, relPath)
	if err != nil {
		return err
	}
	if !hasChanges {
		return nil
	}

	commitMessage := fmt.Sprintf("Add daily Herb Hub post for %s", day.Format("2006-01-02"))
	if err := p.run(ctx, nil, "commit generated post",
		"git", "-C", repoDir,
		"-c", "user.name="+p.config.AuthorName,
		"-c", "user.email="+p.config.AuthorEmail,
		"commit", "-m", commitMessage, "--", relPath,
	); err != nil {
		return err
	}

	pushURL, authEnv, err := p.pushTarget(ctx, repoDir)
	if err != nil {
		return err
	}

	if err := p.run(ctx, authEnv, "push generated post", "git", "-C", repoDir, "push", pushURL, "HEAD:"+p.config.PushBranch); err != nil {
		return err
	}

	return nil
}

func (p *Publisher) hasStagedChanges(ctx context.Context, repoDir, relPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "diff", "--cached", "--quiet", "--", relPath)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("check staged changes: %w", err)
	}
	return false, nil
}

func (p *Publisher) pushTarget(ctx context.Context, repoDir string) (string, []string, error) {
	if strings.TrimSpace(p.config.PAT) == "" {
		return "", nil, fmt.Errorf("GIT_PAT is required when GIT_PUBLISH_ENABLED=true")
	}

	remoteURL, err := p.capture(ctx, nil, "resolve git remote", "git", "-C", repoDir, "remote", "get-url", p.config.RemoteName)
	if err != nil {
		return "", nil, err
	}

	pushURL, err := normalizeGitHubURL(strings.TrimSpace(remoteURL))
	if err != nil {
		return "", nil, err
	}

	parsed, err := url.Parse(pushURL)
	if err != nil {
		return "", nil, fmt.Errorf("parse push url: %w", err)
	}

	headerKey := fmt.Sprintf("http.%s://%s/.extraheader", parsed.Scheme, parsed.Host)
	token := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + p.config.PAT))
	authEnv := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=" + headerKey,
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic " + token,
	}

	return pushURL, authEnv, nil
}

func normalizeGitHubURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "git@github.com:") {
		return "https://github.com/" + strings.TrimPrefix(trimmed, "git@github.com:"), nil
	}
	if strings.HasPrefix(trimmed, "ssh://git@github.com/") {
		return "https://github.com/" + strings.TrimPrefix(trimmed, "ssh://git@github.com/"), nil
	}
	if strings.HasPrefix(trimmed, "https://github.com/") || strings.HasPrefix(trimmed, "http://github.com/") {
		return trimmed, nil
	}
	return "", fmt.Errorf("unsupported git remote for PAT push: %s", trimmed)
}

func (p *Publisher) run(ctx context.Context, extraEnv []string, description string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s: %s", description, message)
	}
	return nil
}

func (p *Publisher) capture(ctx context.Context, extraEnv []string, description string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s: %s", description, message)
	}
	return strings.TrimSpace(string(output)), nil
}
