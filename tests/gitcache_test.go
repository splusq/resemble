package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/splusq/resemble/internal/gitcache"
	"github.com/splusq/resemble/internal/testutil"
)

func TestGitCache_CloneAndPush(t *testing.T) {
	bareRepo := testutil.BareGitRepo(t)
	localDir := testutil.TempDir(t)
	cloneDir := filepath.Join(localDir, "cache")

	gc := gitcache.New(bareRepo, cloneDir)

	// Clone
	if err := gc.Clone(); err != nil {
		t.Fatal(err)
	}

	// Verify clone
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		t.Fatal("expected .git directory after clone")
	}

	// Create a file
	cacheDir := filepath.Join(cloneDir, "cache", "test")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "test.meta.json"), []byte(`{"key":"test"}`), 0644)

	// Commit and push
	if err := gc.CommitAndPush("test commit"); err != nil {
		t.Fatal(err)
	}
}

func TestGitCache_CloneIdempotent(t *testing.T) {
	bareRepo := testutil.BareGitRepo(t)
	localDir := testutil.TempDir(t)
	cloneDir := filepath.Join(localDir, "cache")

	gc := gitcache.New(bareRepo, cloneDir)

	if err := gc.Clone(); err != nil {
		t.Fatal(err)
	}

	// Clone again should not fail
	if err := gc.Clone(); err != nil {
		t.Fatal(err)
	}
}

func TestGitCache_HasChanges(t *testing.T) {
	bareRepo := testutil.BareGitRepo(t)
	localDir := testutil.TempDir(t)
	cloneDir := filepath.Join(localDir, "cache")

	gc := gitcache.New(bareRepo, cloneDir)
	gc.Clone()

	// No changes initially
	hasChanges, err := gc.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if hasChanges {
		t.Error("expected no changes initially")
	}

	// Add a file
	os.WriteFile(filepath.Join(cloneDir, "newfile.txt"), []byte("data"), 0644)

	hasChanges, err = gc.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if !hasChanges {
		t.Error("expected changes after adding file")
	}
}

func TestGitCache_NoChangesSkipsCommit(t *testing.T) {
	bareRepo := testutil.BareGitRepo(t)
	localDir := testutil.TempDir(t)
	cloneDir := filepath.Join(localDir, "cache")

	gc := gitcache.New(bareRepo, cloneDir)
	gc.Clone()

	// Commit with no changes should not error
	if err := gc.CommitAndPush("nothing to commit"); err != nil {
		t.Fatal(err)
	}
}

func TestGitCache_TwoInstancesNonConflicting(t *testing.T) {
	bareRepo := testutil.BareGitRepo(t)

	// Instance 1
	dir1 := filepath.Join(testutil.TempDir(t), "cache1")
	gc1 := gitcache.New(bareRepo, dir1)
	gc1.Clone()

	// Instance 2
	dir2 := filepath.Join(testutil.TempDir(t), "cache2")
	gc2 := gitcache.New(bareRepo, dir2)
	gc2.Clone()

	// Instance 1 writes file A
	os.MkdirAll(filepath.Join(dir1, "cache"), 0755)
	os.WriteFile(filepath.Join(dir1, "cache", "fileA.json"), []byte("A"), 0644)
	if err := gc1.CommitAndPush("add A"); err != nil {
		t.Fatal(err)
	}

	// Instance 2 writes file B (different file, no conflict)
	os.MkdirAll(filepath.Join(dir2, "cache"), 0755)
	os.WriteFile(filepath.Join(dir2, "cache", "fileB.json"), []byte("B"), 0644)
	if err := gc2.CommitAndPush("add B"); err != nil {
		t.Fatal(err)
	}
}
