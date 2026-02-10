package gitcache

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitCache struct {
	repoURL   string
	localPath string
}

func New(repoURL, localPath string) *GitCache {
	return &GitCache{
		repoURL:   repoURL,
		localPath: localPath,
	}
}

func (g *GitCache) Path() string {
	return g.localPath
}

func (g *GitCache) Clone() error {
	if _, err := os.Stat(filepath.Join(g.localPath, ".git")); err == nil {
		log.Printf("resemble: cache repo already cloned at %s", g.localPath)
		return nil
	}

	log.Printf("resemble: cloning cache repo %s -> %s", g.repoURL, g.localPath)
	return g.run("git", "clone", g.repoURL, g.localPath)
}

func (g *GitCache) Pull() error {
	if !g.isGitRepo() {
		return nil
	}
	log.Printf("resemble: pulling latest cache")
	err := g.runInRepo("git", "pull", "--rebase", "--autostash")
	if err != nil {
		// Pull might fail if repo is empty (no commits yet), that's ok
		log.Printf("resemble: pull warning (may be empty repo): %v", err)
	}
	return nil
}

func (g *GitCache) CommitAndPush(message string) error {
	if !g.isGitRepo() {
		return nil
	}

	// Stage all changes in cache dir
	if err := g.runInRepo("git", "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Check if there are changes to commit
	if err := g.runInRepo("git", "diff", "--cached", "--quiet"); err == nil {
		log.Printf("resemble: no changes to commit")
		return nil
	}

	if err := g.runInRepo("git", "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Push with retry on conflict
	for attempt := 0; attempt < 3; attempt++ {
		err := g.runInRepo("git", "push")
		if err == nil {
			log.Printf("resemble: pushed cache changes")
			return nil
		}

		log.Printf("resemble: push failed (attempt %d/3), pulling and retrying: %v", attempt+1, err)
		if pullErr := g.runInRepo("git", "pull", "--rebase"); pullErr != nil {
			return fmt.Errorf("git pull --rebase: %w", pullErr)
		}
	}

	return fmt.Errorf("git push failed after 3 attempts")
}

func (g *GitCache) HasChanges() (bool, error) {
	if !g.isGitRepo() {
		return false, nil
	}
	err := g.runInRepo("git", "diff", "--quiet")
	if err != nil {
		return true, nil // has changes
	}
	// Also check for untracked files
	out, execErr := g.outputInRepo("git", "status", "--porcelain")
	if execErr != nil {
		return false, execErr
	}
	return strings.TrimSpace(out) != "", nil
}

func (g *GitCache) Init() error {
	if err := os.MkdirAll(g.localPath, 0755); err != nil {
		return err
	}
	return g.runInRepo("git", "init")
}

func (g *GitCache) isGitRepo() bool {
	_, err := os.Stat(filepath.Join(g.localPath, ".git"))
	return err == nil
}

func (g *GitCache) run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitCache) runInRepo(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = g.localPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *GitCache) outputInRepo(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = g.localPath
	out, err := cmd.Output()
	return string(out), err
}
