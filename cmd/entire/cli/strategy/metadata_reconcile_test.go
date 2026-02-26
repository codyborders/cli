package strategy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/paths"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestReconcileDisconnected_NoRemote(t *testing.T) {
	t.Parallel()

	// Local-only repo with metadata branch, no remote tracking branch
	tmpDir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	// Create orphan metadata branch
	run("checkout", "--orphan", paths.MetadataBranchName)
	run("rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte(`{"test":true}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "checkpoint")
	run("checkout", "main")

	repo, err := git.PlainOpenWithOptions(tmpDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	// Should be a no-op (no remote)
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileDisconnected_NoLocal(t *testing.T) {
	t.Parallel()

	// Clone from bare with remote metadata but no local metadata branch
	bareDir := initBareWithMetadataBranch(t)
	cloneDir, _ := cloneWithConfig(t, bareDir)

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	// No local branch → no-op
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileDisconnected_SameHash(t *testing.T) {
	t.Parallel()

	bareDir := initBareWithMetadataBranch(t)
	cloneDir, _ := cloneWithConfig(t, bareDir)

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	// Create local branch from remote (same hash)
	if err := EnsureMetadataBranch(repo); err != nil {
		t.Fatalf("EnsureMetadataBranch failed: %v", err)
	}

	// Same hash → no-op
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileDisconnected_SharedAncestry(t *testing.T) {
	t.Parallel()

	bareDir := initBareWithMetadataBranch(t)
	cloneDir, run := cloneWithConfig(t, bareDir)

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	// Create local branch from remote (shared base)
	if err := EnsureMetadataBranch(repo); err != nil {
		t.Fatalf("EnsureMetadataBranch failed: %v", err)
	}

	// Add a local commit on top (diverged, but shared ancestry)
	run("checkout", paths.MetadataBranchName)
	localDir := filepath.Join(cloneDir, "cd", "ef01234567")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "metadata.json"), []byte(`{"test":"local"}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "local checkpoint")
	run("checkout", "main")

	// Re-open to see updated refs
	repo, err = git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to re-open repo: %v", err)
	}

	// Shared ancestry → no-op
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileDisconnected_Disconnected(t *testing.T) {
	t.Parallel()

	bareDir := initBareWithMetadataBranch(t)
	cloneDir, run := cloneWithConfig(t, bareDir)

	// Create a disconnected local metadata branch (simulating the empty-orphan bug)
	run("checkout", "--orphan", "temp-orphan")
	run("rm", "-rf", ".")
	localDir := filepath.Join(cloneDir, "ab", "cdef012345")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "metadata.json"), []byte(`{"checkpoint_id":"abcdef012345"}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: abcdef012345")
	run("branch", "-f", paths.MetadataBranchName, "temp-orphan")
	run("checkout", "main")

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	// Verify they are disconnected before reconcile
	refName := plumbing.NewBranchReferenceName(paths.MetadataBranchName)
	localRef, err := repo.Reference(refName, true)
	if err != nil {
		t.Fatalf("local ref not found: %v", err)
	}
	remoteRefName := plumbing.NewRemoteReferenceName("origin", paths.MetadataBranchName)
	remoteRef, err := repo.Reference(remoteRefName, true)
	if err != nil {
		t.Fatalf("remote ref not found: %v", err)
	}
	if localRef.Hash() == remoteRef.Hash() {
		t.Fatal("expected different hashes before reconcile")
	}

	// Run reconciliation
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("ReconcileDisconnectedMetadataBranch() failed: %v", err)
	}

	// Verify result
	newRef, err := repo.Reference(refName, true)
	if err != nil {
		t.Fatalf("local ref not found after reconcile: %v", err)
	}

	// Should have linear history: new tip -> remote tip -> remote root
	tipCommit, err := repo.CommitObject(newRef.Hash())
	if err != nil {
		t.Fatalf("failed to get tip commit: %v", err)
	}

	// Tip's parent should be the remote tip (linear chain, not merge)
	if len(tipCommit.ParentHashes) != 1 {
		t.Fatalf("expected 1 parent (linear), got %d", len(tipCommit.ParentHashes))
	}
	if tipCommit.ParentHashes[0] != remoteRef.Hash() {
		t.Errorf("tip parent = %s, want remote tip %s", tipCommit.ParentHashes[0], remoteRef.Hash())
	}

	// Verify merged tree contains both local and remote data
	tree, err := tipCommit.Tree()
	if err != nil {
		t.Fatalf("failed to get tree: %v", err)
	}

	entries := make(map[string]object.TreeEntry)
	if err := checkpoint.FlattenTree(repo, tree, "", entries); err != nil {
		t.Fatalf("failed to flatten tree: %v", err)
	}

	// Remote data: metadata.json at root (from initBareWithMetadataBranch)
	if _, ok := entries["metadata.json"]; !ok {
		t.Error("merged tree missing remote data (metadata.json)")
	}
	// Local data: ab/cdef012345/metadata.json
	if _, ok := entries["ab/cdef012345/metadata.json"]; !ok {
		t.Error("merged tree missing local data (ab/cdef012345/metadata.json)")
	}

	// Original commit message should be preserved (git adds trailing newline)
	if tipCommit.Message != "Checkpoint: abcdef012345\n" {
		t.Errorf("commit message not preserved: got %q", tipCommit.Message)
	}
}

func TestReconcileDisconnected_MultipleLocalCheckpoints(t *testing.T) {
	t.Parallel()

	bareDir := initBareWithMetadataBranch(t)
	cloneDir, run := cloneWithConfig(t, bareDir)

	// Create a disconnected local branch with 3 commits (empty root + 3 data commits)
	run("checkout", "--orphan", "temp-orphan")
	run("rm", "-rf", ".")

	// Empty root commit (the orphan bug commit)
	run("commit", "--allow-empty", "-m", "Initialize metadata branch")

	// Checkpoint 1
	dir1 := filepath.Join(cloneDir, "11", "1111111111")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "metadata.json"), []byte(`{"checkpoint_id":"111111111111"}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: 111111111111")

	// Checkpoint 2
	dir2 := filepath.Join(cloneDir, "22", "2222222222")
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "metadata.json"), []byte(`{"checkpoint_id":"222222222222"}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: 222222222222")

	// Checkpoint 3
	dir3 := filepath.Join(cloneDir, "33", "3333333333")
	if err := os.MkdirAll(dir3, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir3, "metadata.json"), []byte(`{"checkpoint_id":"333333333333"}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: 333333333333")

	run("branch", "-f", paths.MetadataBranchName, "temp-orphan")
	run("checkout", "main")

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	remoteRefName := plumbing.NewRemoteReferenceName("origin", paths.MetadataBranchName)
	remoteRef, err := repo.Reference(remoteRefName, true)
	if err != nil {
		t.Fatalf("remote ref not found: %v", err)
	}

	// Run reconciliation
	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("ReconcileDisconnectedMetadataBranch() failed: %v", err)
	}

	// Verify result
	refName := plumbing.NewBranchReferenceName(paths.MetadataBranchName)
	newRef, err := repo.Reference(refName, true)
	if err != nil {
		t.Fatalf("local ref not found after reconcile: %v", err)
	}

	// Walk commits to verify linear chain
	var commitMessages []string
	current := newRef.Hash()
	for range 10 {
		c, cErr := repo.CommitObject(current)
		if cErr != nil {
			t.Fatalf("failed to get commit %s: %v", current, cErr)
		}
		commitMessages = append(commitMessages, c.Message)
		if len(c.ParentHashes) == 0 {
			break
		}
		if len(c.ParentHashes) != 1 {
			t.Fatalf("expected linear history, commit %s has %d parents", c.Hash, len(c.ParentHashes))
		}
		current = c.ParentHashes[0]
	}

	// Should have: 3 cherry-picked + remote commits (1 data + 1 root = at least 2)
	// The empty orphan commit is skipped, so we get exactly 3 cherry-picked commits
	if len(commitMessages) < 4 {
		t.Errorf("expected at least 4 commits in chain, got %d: %v", len(commitMessages), commitMessages)
	}

	// Verify all checkpoint data is in the final tree
	tipCommit, err := repo.CommitObject(newRef.Hash())
	if err != nil {
		t.Fatalf("failed to get tip: %v", err)
	}
	tree, err := tipCommit.Tree()
	if err != nil {
		t.Fatalf("failed to get tree: %v", err)
	}

	entries := make(map[string]object.TreeEntry)
	if err := checkpoint.FlattenTree(repo, tree, "", entries); err != nil {
		t.Fatalf("failed to flatten tree: %v", err)
	}

	expectedPaths := []string{
		"metadata.json",               // Remote data
		"11/1111111111/metadata.json", // Checkpoint 1
		"22/2222222222/metadata.json", // Checkpoint 2
		"33/3333333333/metadata.json", // Checkpoint 3
	}
	for _, p := range expectedPaths {
		if _, ok := entries[p]; !ok {
			t.Errorf("merged tree missing expected path: %s", p)
		}
	}

	// First cherry-picked commit's parent should be the remote tip
	// Walk back from tip: tip (cp3) -> cp2 -> cp1 -> remote tip
	cp3, err := repo.CommitObject(newRef.Hash())
	if err != nil {
		t.Fatalf("failed to get cp3: %v", err)
	}
	cp2, err := repo.CommitObject(cp3.ParentHashes[0])
	if err != nil {
		t.Fatalf("failed to get cp2: %v", err)
	}
	cp1, err := repo.CommitObject(cp2.ParentHashes[0])
	if err != nil {
		t.Fatalf("failed to get cp1: %v", err)
	}
	if cp1.ParentHashes[0] != remoteRef.Hash() {
		t.Errorf("first cherry-picked commit parent = %s, want remote tip %s",
			cp1.ParentHashes[0], remoteRef.Hash())
	}
}

func TestReconcileDisconnected_ModifiedEntries(t *testing.T) {
	t.Parallel()

	bareDir := initBareWithMetadataBranch(t)
	cloneDir, run := cloneWithConfig(t, bareDir)

	// Create a disconnected local branch where commit 2 modifies a file from commit 1
	// (simulates multi-session condensation updating metadata.json)
	run("checkout", "--orphan", "temp-orphan")
	run("rm", "-rf", ".")

	// Commit 1: initial checkpoint
	dir1 := filepath.Join(cloneDir, "aa", "aaaaaaaaaa")
	if err := os.MkdirAll(dir1, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "metadata.json"),
		[]byte(`{"checkpoint_id":"aaaaaaaaaaaa","session_count":1}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: aaaaaaaaaaaa")

	// Commit 2: update same checkpoint (session_count 1→2) + add new file
	if err := os.WriteFile(filepath.Join(dir1, "metadata.json"),
		[]byte(`{"checkpoint_id":"aaaaaaaaaaaa","session_count":2}`), 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "1", "metadata.json"),
		[]byte(`{"session_id":"second-session"}`), 0o644); err != nil {
		if err := os.MkdirAll(filepath.Join(dir1, "1"), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir1, "1", "metadata.json"),
			[]byte(`{"session_id":"second-session"}`), 0o644); err != nil {
			t.Fatalf("failed to write: %v", err)
		}
	}
	run("add", ".")
	run("commit", "-m", "Checkpoint: aaaaaaaaaaaa (update)")

	run("branch", "-f", paths.MetadataBranchName, "temp-orphan")
	run("checkout", "main")

	repo, err := git.PlainOpenWithOptions(cloneDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}

	if err := ReconcileDisconnectedMetadataBranch(repo); err != nil {
		t.Fatalf("ReconcileDisconnectedMetadataBranch() failed: %v", err)
	}

	// Verify the MODIFIED metadata.json has session_count:2, not the original 1
	refName := plumbing.NewBranchReferenceName(paths.MetadataBranchName)
	newRef, err := repo.Reference(refName, true)
	if err != nil {
		t.Fatalf("local ref not found: %v", err)
	}
	tipCommit, err := repo.CommitObject(newRef.Hash())
	if err != nil {
		t.Fatalf("failed to get tip: %v", err)
	}
	tree, err := tipCommit.Tree()
	if err != nil {
		t.Fatalf("failed to get tree: %v", err)
	}

	metadataFile, err := tree.File("aa/aaaaaaaaaa/metadata.json")
	if err != nil {
		t.Fatalf("metadata.json not found in tree: %v", err)
	}
	content, err := metadataFile.Contents()
	if err != nil {
		t.Fatalf("failed to read metadata.json: %v", err)
	}
	if !strings.Contains(content, `"session_count":2`) {
		t.Errorf("metadata.json should have session_count:2 (modified value), got: %s", content)
	}
}
