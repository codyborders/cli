package checkpoint

import (
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadGeneration_EmptyTree_ReturnsDefault(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	store := NewV2GitStore(repo)

	// Build an empty tree
	emptyTree, err := BuildTreeFromEntries(repo, map[string]object.TreeEntry{})
	require.NoError(t, err)

	gen, err := store.readGeneration(emptyTree)
	require.NoError(t, err)

	assert.Equal(t, 0, gen.Generation)
	assert.Equal(t, 0, gen.CheckpointCount)
	assert.Empty(t, gen.Checkpoints)
	assert.True(t, gen.OldestCheckpointAt.IsZero())
	assert.True(t, gen.NewestCheckpointAt.IsZero())
}

func TestReadGeneration_ParsesJSON(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	store := NewV2GitStore(repo)

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	original := GenerationMetadata{
		Generation:         1,
		CheckpointCount:    2,
		Checkpoints:        []string{"aabbccddeeff", "112233445566"},
		OldestCheckpointAt: now.Add(-1 * time.Hour),
		NewestCheckpointAt: now,
	}

	// Write generation.json into a tree
	entries := make(map[string]object.TreeEntry)
	require.NoError(t, store.writeGeneration(original, entries))

	treeHash, err := BuildTreeFromEntries(repo, entries)
	require.NoError(t, err)

	// Read it back
	gen, err := store.readGeneration(treeHash)
	require.NoError(t, err)

	assert.Equal(t, 1, gen.Generation)
	assert.Equal(t, 2, gen.CheckpointCount)
	assert.Equal(t, []string{"aabbccddeeff", "112233445566"}, gen.Checkpoints)
	assert.True(t, gen.OldestCheckpointAt.Equal(now.Add(-1*time.Hour)))
	assert.True(t, gen.NewestCheckpointAt.Equal(now))
}

func TestWriteGeneration_RoundTrips(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	store := NewV2GitStore(repo)

	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	original := GenerationMetadata{
		Generation:         0,
		CheckpointCount:    1,
		Checkpoints:        []string{"aabbccddeeff"},
		OldestCheckpointAt: now,
		NewestCheckpointAt: now,
	}

	entries := make(map[string]object.TreeEntry)
	require.NoError(t, store.writeGeneration(original, entries))

	// Verify the entry was added at the right key
	_, ok := entries[paths.GenerationFileName]
	assert.True(t, ok)

	// Build tree and read back
	treeHash, err := BuildTreeFromEntries(repo, entries)
	require.NoError(t, err)

	gen, err := store.readGeneration(treeHash)
	require.NoError(t, err)

	assert.Equal(t, original.Generation, gen.Generation)
	assert.Equal(t, original.CheckpointCount, gen.CheckpointCount)
	assert.Equal(t, original.Checkpoints, gen.Checkpoints)
}

func TestReadGenerationFromRef(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	store := NewV2GitStore(repo)

	// Create a ref with generation.json in its tree
	now := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)
	gen := GenerationMetadata{
		Generation:         0,
		CheckpointCount:    1,
		Checkpoints:        []string{"aabbccddeeff"},
		OldestCheckpointAt: now,
		NewestCheckpointAt: now,
	}

	entries := make(map[string]object.TreeEntry)
	require.NoError(t, store.writeGeneration(gen, entries))
	treeHash, err := BuildTreeFromEntries(repo, entries)
	require.NoError(t, err)

	refName := plumbing.ReferenceName(paths.V2FullCurrentRefName)
	authorName, authorEmail := GetGitAuthorFromRepo(repo)
	commitHash, err := CreateCommit(repo, treeHash, plumbing.ZeroHash, "test", authorName, authorEmail)
	require.NoError(t, err)
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(refName, commitHash)))

	// Read back via ref
	result, err := store.readGenerationFromRef(refName)
	require.NoError(t, err)

	assert.Equal(t, 0, result.Generation)
	assert.Equal(t, 1, result.CheckpointCount)
	assert.Equal(t, []string{"aabbccddeeff"}, result.Checkpoints)
}

func TestAddGenerationToRootTree(t *testing.T) {
	t.Parallel()
	repo := initTestRepo(t)
	store := NewV2GitStore(repo)

	// Start with a root tree that has a shard directory entry (simulating checkpoint data)
	shardEntries := map[string]object.TreeEntry{}
	shardEntries["aa/bbccddeeff/0/full.jsonl"] = object.TreeEntry{
		Name: "full.jsonl",
		Mode: 0o100644,
		Hash: plumbing.ZeroHash, // dummy
	}
	rootTreeHash, err := BuildTreeFromEntries(repo, shardEntries)
	require.NoError(t, err)

	gen := GenerationMetadata{
		Generation:      0,
		CheckpointCount: 1,
		Checkpoints:     []string{"aabbccddeeff"},
	}

	// Add generation.json to the root tree
	newRootHash, err := store.addGenerationToRootTree(rootTreeHash, gen)
	require.NoError(t, err)
	assert.NotEqual(t, rootTreeHash, newRootHash)

	// Verify generation.json is present and shard dir is preserved
	readGen, err := store.readGeneration(newRootHash)
	require.NoError(t, err)
	assert.Equal(t, 1, readGen.CheckpointCount)

	// Verify the shard directory still exists in the tree
	tree, err := repo.TreeObject(newRootHash)
	require.NoError(t, err)
	foundShard := false
	for _, e := range tree.Entries {
		if e.Name == "aa" {
			foundShard = true
		}
	}
	assert.True(t, foundShard, "shard directory should be preserved")
}
