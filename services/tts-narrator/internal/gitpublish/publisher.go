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

	"HerbHub365/services/tts-narrator/internal/config"
)

// Result carries the paths that need to be committed.
type Result struct {
	// PostPath is the (possibly patched) Jekyll post file.
	PostPath string
	// AudioPath is the generated MP3 file.
	AudioPath string
}

// Publisher commits and pushes narration results to the git repo.
type Publisher struct {
	config config.GitConfig
}

// NewPublisher constructs a Publisher.
func NewPublisher(cfg config.GitConfig) *Publisher {
	return &Publisher{config: cfg}
}

// PublishNarration stages the patched post and new MP3, commits, and pushes.
func (p *Publisher) PublishNarration(ctx context.Context, result Result, day time.Time) error {
	if !p.config.PublishEnabled {
		return nil
	}

	repoDir, err := filepath.Abs(p.config.RepoDir)
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}

	// Build repo-relative paths for the post and the audio file.
	pathsToAdd := make([]string, 0, 2)
	for _, abs := range []string{result.PostPath, result.AudioPath} {
		if abs == "" {
			continue
		}
		absPath, err := filepath.Abs(abs)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(repoDir, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("path %s is outside git repo %s", abs, repoDir)
		}
		pathsToAdd = append(pathsToAdd, rel)
	}

	gitEnv := p.gitEnv(repoDir)

	addArgs := append([]string{"-C", repoDir, "add", "--"}, pathsToAdd...)
	if err := p.run(ctx, gitEnv, "git add narration files", "git", addArgs...); err != nil {
		return err
	}

	hasChanges, err := p.hasStagedChanges(ctx, repoDir, pathsToAdd...)
	if err != nil {
		return err
	}
	if !hasChanges {
		return nil
	}

	commitMsg := fmt.Sprintf("Add TTS narration for %s", day.Format("2006-01-02"))
	if err := p.run(ctx, gitEnv, "git commit narration",
		"git", "-C", repoDir,
		"-c", "user.name="+p.config.AuthorName,
		"-c", "user.email="+p.config.AuthorEmail,
		"commit", "-m", commitMsg,
	); err != nil {
		return err
	}

	pushURL, authEnv, err := p.pushTarget(ctx, repoDir)
	if err != nil {
		return err
	}

	return p.run(ctx, authEnv, "git push narration", "git", "-C", repoDir, "push", pushURL, "HEAD:"+p.config.PushBranch)
}

func (p *Publisher) hasStagedChanges(ctx context.Context, repoDir string, relPaths ...string) (bool, error) {
	args := []string{"-C", repoDir, "diff", "--cached", "--quiet", "--"}
	args = append(args, relPaths...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(cmd.Environ(), p.gitEnv(repoDir)...)
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

	remoteURL, err := p.capture(ctx, p.gitEnv(repoDir), "resolve git remote",
		"git", "-C", repoDir, "remote", "get-url", p.config.RemoteName)
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
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=" + repoDir,
		"GIT_CONFIG_KEY_1=" + headerKey,
		"GIT_CONFIG_VALUE_1=AUTHORIZATION: basic " + token,
	}
	return pushURL, authEnv, nil
}

func (p *Publisher) gitEnv(repoDir string) []string {
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=" + repoDir,
	}
}

func normalizeGitHubURL(value string) (string, error) {
	t := strings.TrimSpace(value)
	if strings.HasPrefix(t, "git@github.com:") {
		return "https://github.com/" + strings.TrimPrefix(t, "git@github.com:"), nil
	}
	if strings.HasPrefix(t, "ssh://git@github.com/") {
		return "https://github.com/" + strings.TrimPrefix(t, "ssh://git@github.com/"), nil
	}
	if strings.HasPrefix(t, "https://github.com/") || strings.HasPrefix(t, "http://github.com/") {
		return t, nil
	}
	return "", fmt.Errorf("unsupported git remote for PAT push: %s", t)
}

func (p *Publisher) run(ctx context.Context, extraEnv []string, description, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", description, msg)
	}
	return nil
}

func (p *Publisher) capture(ctx context.Context, extraEnv []string, description, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", description, msg)
	}
	return strings.TrimSpace(string(out)), nil
}
