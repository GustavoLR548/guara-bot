package storage

import (
	"testing"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGitHubTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return client, mr
}

func TestGitHubRepository_RegisterAndGet(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	testRepo := github.Repository{
		ID:           "test-repo",
		Owner:        "testowner",
		Name:         "testrepo",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}

	// Register repository
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Get repository
	retrieved, err := repo.GetRepository("test-repo")
	require.NoError(t, err)
	assert.Equal(t, testRepo.ID, retrieved.ID)
	assert.Equal(t, testRepo.Owner, retrieved.Owner)
	assert.Equal(t, testRepo.Name, retrieved.Name)
	assert.Equal(t, testRepo.TargetBranch, retrieved.TargetBranch)
}

func TestGitHubRepository_HasRepository(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Check non-existent repository
	has, err := repo.HasRepository("nonexistent")
	require.NoError(t, err)
	assert.False(t, has)

	// Register and check
	testRepo := github.Repository{
		ID:           "exists",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}
	err = repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	has, err = repo.HasRepository("exists")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestGitHubRepository_UnregisterRepository(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	testRepo := github.Repository{
		ID:           "to-delete",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}

	// Register
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Add a channel
	err = repo.AddRepoChannel("to-delete", "channel123")
	require.NoError(t, err)

	// Unregister
	err = repo.UnregisterRepository("to-delete")
	require.NoError(t, err)

	// Verify deletion
	has, err := repo.HasRepository("to-delete")
	require.NoError(t, err)
	assert.False(t, has)

	// Verify channel association is also deleted
	channels, err := repo.GetRepoChannels("to-delete")
	require.NoError(t, err)
	assert.Empty(t, channels)
}

func TestGitHubRepository_GetAllRepositories(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register multiple repositories
	repos := []github.Repository{
		{ID: "repo1", Owner: "owner1", Name: "name1", TargetBranch: "main", AddedAt: time.Now()},
		{ID: "repo2", Owner: "owner2", Name: "name2", TargetBranch: "develop", AddedAt: time.Now()},
		{ID: "repo3", Owner: "owner3", Name: "name3", TargetBranch: "main", AddedAt: time.Now()},
	}

	for _, r := range repos {
		err := repo.RegisterRepository(r)
		require.NoError(t, err)
	}

	// Get all
	all, err := repo.GetAllRepositories()
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Verify all repos are present
	ids := make(map[string]bool)
	for _, r := range all {
		ids[r.ID] = true
	}
	assert.True(t, ids["repo1"])
	assert.True(t, ids["repo2"])
	assert.True(t, ids["repo3"])
}

func TestGitHubRepository_ChannelAssociations(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register repository
	testRepo := github.Repository{
		ID:           "test-repo",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Add channels
	err = repo.AddRepoChannel("test-repo", "channel1")
	require.NoError(t, err)
	err = repo.AddRepoChannel("test-repo", "channel2")
	require.NoError(t, err)
	err = repo.AddRepoChannel("test-repo", "channel3")
	require.NoError(t, err)

	// Get repo channels
	channels, err := repo.GetRepoChannels("test-repo")
	require.NoError(t, err)
	assert.Len(t, channels, 3)
	assert.Contains(t, channels, "channel1")
	assert.Contains(t, channels, "channel2")
	assert.Contains(t, channels, "channel3")

	// Get channel repos
	repos, err := repo.GetChannelRepos("channel1")
	require.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Contains(t, repos, "test-repo")

	// Remove one channel
	err = repo.RemoveRepoChannel("test-repo", "channel2")
	require.NoError(t, err)

	channels, err = repo.GetRepoChannels("test-repo")
	require.NoError(t, err)
	assert.Len(t, channels, 2)
	assert.NotContains(t, channels, "channel2")
}

func TestGitHubRepository_Deduplication(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register repository
	testRepo := github.Repository{
		ID:           "test-repo",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Check non-processed PR
	processed, err := repo.IsProcessed("test-repo", 123)
	require.NoError(t, err)
	assert.False(t, processed)

	// Mark as processed
	err = repo.MarkProcessed("test-repo", 123)
	require.NoError(t, err)

	// Check again
	processed, err = repo.IsProcessed("test-repo", 123)
	require.NoError(t, err)
	assert.True(t, processed)

	// Different PR should not be processed
	processed, err = repo.IsProcessed("test-repo", 456)
	require.NoError(t, err)
	assert.False(t, processed)
}

func TestGitHubRepository_PendingQueue(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register repository
	testRepo := github.Repository{
		ID:           "test-repo",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Add PRs to pending queue
	mergedAt := time.Now()
	pr1 := github.PullRequest{
		ID:       1,
		Number:   1,
		Title:    "First PR",
		Body:     "Description 1",
		HTMLURL:  "https://github.com/owner/name/pull/1",
		MergedAt: &mergedAt,
		Author:   "user1",
		Labels:   []github.Label{{Name: "feature"}},
	}
	pr2 := github.PullRequest{
		ID:       2,
		Number:   2,
		Title:    "Second PR",
		Body:     "Description 2",
		HTMLURL:  "https://github.com/owner/name/pull/2",
		MergedAt: &mergedAt,
		Author:   "user2",
		Labels:   []github.Label{{Name: "bug"}},
	}

	err = repo.AddToPendingQueue("test-repo", pr1)
	require.NoError(t, err)
	err = repo.AddToPendingQueue("test-repo", pr2)
	require.NoError(t, err)

	// Check pending count
	count, err := repo.GetPendingCount("test-repo")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Get pending queue
	prs, err := repo.GetPendingQueue("test-repo")
	require.NoError(t, err)
	assert.Len(t, prs, 2)
	assert.Equal(t, int64(1), prs[0].ID)
	assert.Equal(t, int64(2), prs[1].ID)

	// Clear pending queue
	err = repo.ClearPendingQueue("test-repo")
	require.NoError(t, err)

	count, err = repo.GetPendingCount("test-repo")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	prs, err = repo.GetPendingQueue("test-repo")
	require.NoError(t, err)
	assert.Empty(t, prs)
}

func TestGitHubRepository_LastChecked(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register repository
	testRepo := github.Repository{
		ID:           "test-repo",
		Owner:        "owner",
		Name:         "name",
		TargetBranch: "main",
		AddedAt:      time.Now(),
	}
	err := repo.RegisterRepository(testRepo)
	require.NoError(t, err)

	// Get initial last checked (should be zero)
	lastChecked, err := repo.GetLastChecked("test-repo")
	require.NoError(t, err)
	assert.True(t, lastChecked.IsZero())

	// Update last checked
	now := time.Now()
	err = repo.UpdateLastChecked("test-repo", now)
	require.NoError(t, err)

	// Get updated last checked
	lastChecked, err = repo.GetLastChecked("test-repo")
	require.NoError(t, err)
	// Allow for small time difference due to serialization
	assert.WithinDuration(t, now, lastChecked, time.Second)
}

func TestGitHubRepository_ManyToManyAssociations(t *testing.T) {
	client, mr := setupGitHubTestRedis(t)
	defer mr.Close()
	defer client.Close()

	repo := NewRedisGitHubRepository(client)

	// Register multiple repositories
	repo1 := github.Repository{ID: "repo1", Owner: "owner1", Name: "name1", TargetBranch: "main", AddedAt: time.Now()}
	repo2 := github.Repository{ID: "repo2", Owner: "owner2", Name: "name2", TargetBranch: "main", AddedAt: time.Now()}
	err := repo.RegisterRepository(repo1)
	require.NoError(t, err)
	err = repo.RegisterRepository(repo2)
	require.NoError(t, err)

	// Associate multiple repos with channel1
	err = repo.AddRepoChannel("repo1", "channel1")
	require.NoError(t, err)
	err = repo.AddRepoChannel("repo2", "channel1")
	require.NoError(t, err)

	// Associate repo1 with multiple channels
	err = repo.AddRepoChannel("repo1", "channel2")
	require.NoError(t, err)

	// Verify many-to-many relationships
	repos, err := repo.GetChannelRepos("channel1")
	require.NoError(t, err)
	assert.Len(t, repos, 2)
	assert.Contains(t, repos, "repo1")
	assert.Contains(t, repos, "repo2")

	channels, err := repo.GetRepoChannels("repo1")
	require.NoError(t, err)
	assert.Len(t, channels, 2)
	assert.Contains(t, channels, "channel1")
	assert.Contains(t, channels, "channel2")

	channels, err = repo.GetRepoChannels("repo2")
	require.NoError(t, err)
	assert.Len(t, channels, 1)
	assert.Contains(t, channels, "channel1")
}
