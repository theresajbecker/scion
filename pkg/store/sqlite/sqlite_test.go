// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !no_sqlite

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/ptone/scion-agent/pkg/api"
	"github.com/ptone/scion-agent/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := New(":memory:")
	require.NoError(t, err)

	err = s.Migrate(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

// ============================================================================
// Agent Tests
// ============================================================================

func TestAgentCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// First create a grove for the agent
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create agent
	agent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:       "test-agent",
		Name:       "Test Agent",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusPending,
		Visibility: store.VisibilityPrivate,
		Labels:     map[string]string{"env": "test"},
	}

	err := s.CreateAgent(ctx, agent)
	require.NoError(t, err)
	assert.NotZero(t, agent.Created)
	assert.Equal(t, int64(1), agent.StateVersion)

	// Get agent
	retrieved, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agent.ID, retrieved.ID)
	assert.Equal(t, agent.Slug, retrieved.Slug)
	assert.Equal(t, agent.Name, retrieved.Name)
	assert.Equal(t, agent.Template, retrieved.Template)
	assert.Equal(t, "test", retrieved.Labels["env"])

	// Get by slug
	retrieved, err = s.GetAgentBySlug(ctx, grove.ID, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, agent.ID, retrieved.ID)

	// Update agent
	retrieved.Name = "Updated Agent"
	retrieved.Status = store.AgentStatusRunning
	err = s.UpdateAgent(ctx, retrieved)
	require.NoError(t, err)
	assert.Equal(t, int64(2), retrieved.StateVersion)

	// Verify update
	retrieved, err = s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Agent", retrieved.Name)
	assert.Equal(t, store.AgentStatusRunning, retrieved.Status)

	// Test version conflict
	oldVersion := retrieved.StateVersion
	retrieved.StateVersion = 1 // Use old version
	err = s.UpdateAgent(ctx, retrieved)
	assert.ErrorIs(t, err, store.ErrVersionConflict)

	// Restore correct version for delete
	retrieved.StateVersion = oldVersion

	// Delete agent
	err = s.DeleteAgent(ctx, agent.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = s.GetAgent(ctx, agent.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestAgentList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create multiple agents
	for i := 0; i < 5; i++ {
		agent := &store.Agent{
			ID:         api.NewUUID(),
			Slug:    api.Slugify("agent-" + string(rune('a'+i))),
			Name:       "Agent " + string(rune('A'+i)),
			Template:   "claude",
			GroveID:    grove.ID,
			Status:     store.AgentStatusRunning,
			Visibility: store.VisibilityPrivate,
		}
		if i%2 == 0 {
			agent.Status = store.AgentStatusStopped
		}
		require.NoError(t, s.CreateAgent(ctx, agent))
	}

	// List all
	result, err := s.ListAgents(ctx, store.AgentFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 5, result.TotalCount)
	assert.Len(t, result.Items, 5)

	// List by status
	result, err = s.ListAgents(ctx, store.AgentFilter{Status: store.AgentStatusRunning}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalCount)

	// List by grove
	result, err = s.ListAgents(ctx, store.AgentFilter{GroveID: grove.ID}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 5, result.TotalCount)

	// Test pagination
	result, err = s.ListAgents(ctx, store.AgentFilter{}, store.ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestAgentStatusUpdate(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove and agent
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	agent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:    "test-agent",
		Name:       "Test Agent",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusPending,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	// Update status
	err := s.UpdateAgentStatus(ctx, agent.ID, store.AgentStatusUpdate{
		Status:          store.AgentStatusRunning,
		ContainerStatus: "Up 5 minutes",
	})
	require.NoError(t, err)

	// Verify
	retrieved, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusRunning, retrieved.Status)
	assert.Equal(t, "Up 5 minutes", retrieved.ContainerStatus)
}

func TestSoftDeleteFilterExclusion(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove-sd",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create 3 agents: 2 running, 1 soft-deleted
	for i := 0; i < 3; i++ {
		agent := &store.Agent{
			ID:         api.NewUUID(),
			Slug:       api.Slugify("sd-agent-" + string(rune('a'+i))),
			Name:       "SD Agent " + string(rune('A'+i)),
			Template:   "claude",
			GroveID:    grove.ID,
			Status:     store.AgentStatusRunning,
			Visibility: store.VisibilityPrivate,
		}
		if i == 2 {
			agent.Status = store.AgentStatusDeleted
			agent.DeletedAt = time.Now()
		}
		require.NoError(t, s.CreateAgent(ctx, agent))
	}

	// List without IncludeDeleted: should see 2
	result, err := s.ListAgents(ctx, store.AgentFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalCount)
	assert.Len(t, result.Items, 2)
	for _, a := range result.Items {
		assert.NotEqual(t, store.AgentStatusDeleted, a.Status)
	}

	// List with IncludeDeleted: should see 3
	result, err = s.ListAgents(ctx, store.AgentFilter{IncludeDeleted: true}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)
	assert.Len(t, result.Items, 3)

	// List with Status=deleted: should see 1 (the deleted one)
	result, err = s.ListAgents(ctx, store.AgentFilter{Status: store.AgentStatusDeleted}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, store.AgentStatusDeleted, result.Items[0].Status)
}

func TestPurgeDeletedAgents(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove-purge",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	now := time.Now()

	// Create 2 deleted agents: one expired (old), one recent
	oldAgent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:       "old-deleted",
		Name:       "Old Deleted",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusDeleted,
		DeletedAt:  now.Add(-48 * time.Hour),
		Visibility: store.VisibilityPrivate,
	}
	recentAgent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:       "recent-deleted",
		Name:       "Recent Deleted",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusDeleted,
		DeletedAt:  now.Add(-1 * time.Hour),
		Visibility: store.VisibilityPrivate,
	}
	activeAgent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:       "active-agent",
		Name:       "Active Agent",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, oldAgent))
	require.NoError(t, s.CreateAgent(ctx, recentAgent))
	require.NoError(t, s.CreateAgent(ctx, activeAgent))

	// Purge with cutoff of 24h ago: should only purge the old one
	cutoff := now.Add(-24 * time.Hour)
	purged, err := s.PurgeDeletedAgents(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, purged)

	// Old agent should be gone
	_, err = s.GetAgent(ctx, oldAgent.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// Recent deleted agent should still exist
	_, err = s.GetAgent(ctx, recentAgent.ID)
	require.NoError(t, err)

	// Active agent should still exist
	_, err = s.GetAgent(ctx, activeAgent.ID)
	require.NoError(t, err)
}

func TestDeletedAtPersistence(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove-dat",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create and soft-delete an agent
	agent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:       "soft-del-test",
		Name:       "Soft Delete Test",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	// Verify DeletedAt is zero initially
	retrieved, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.DeletedAt.IsZero())

	// Soft-delete
	deletedAt := time.Now().Truncate(time.Second)
	retrieved.Status = store.AgentStatusDeleted
	retrieved.DeletedAt = deletedAt
	retrieved.Updated = time.Now()
	require.NoError(t, s.UpdateAgent(ctx, retrieved))

	// Retrieve and verify DeletedAt is set
	retrieved2, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusDeleted, retrieved2.Status)
	assert.False(t, retrieved2.DeletedAt.IsZero())
	assert.WithinDuration(t, deletedAt, retrieved2.DeletedAt, time.Second)

	// Verify GetAgentBySlug also returns DeletedAt
	bySlug, err := s.GetAgentBySlug(ctx, grove.ID, "soft-del-test")
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusDeleted, bySlug.Status)
	assert.False(t, bySlug.DeletedAt.IsZero())

	// Verify restore clears DeletedAt
	bySlug.Status = store.AgentStatusRestored
	bySlug.DeletedAt = time.Time{}
	bySlug.Updated = time.Now()
	require.NoError(t, s.UpdateAgent(ctx, bySlug))

	restored, err := s.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusRestored, restored.Status)
	assert.True(t, restored.DeletedAt.IsZero())
}

// ============================================================================
// Grove Tests
// ============================================================================

func TestGroveCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "My Project",
		Slug:       "my-project",
		GitRemote:  "github.com/org/repo",
		Visibility: store.VisibilityPrivate,
		Labels:     map[string]string{"team": "platform"},
	}

	err := s.CreateGrove(ctx, grove)
	require.NoError(t, err)
	assert.NotZero(t, grove.Created)

	// Get grove
	retrieved, err := s.GetGrove(ctx, grove.ID)
	require.NoError(t, err)
	assert.Equal(t, grove.Name, retrieved.Name)
	assert.Equal(t, grove.GitRemote, retrieved.GitRemote)
	assert.Equal(t, "platform", retrieved.Labels["team"])

	// Get by slug
	retrieved, err = s.GetGroveBySlug(ctx, "my-project")
	require.NoError(t, err)
	assert.Equal(t, grove.ID, retrieved.ID)

	// Get by git remote
	retrieved, err = s.GetGroveByGitRemote(ctx, "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, grove.ID, retrieved.ID)

	// Test unique constraint on git remote
	duplicate := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Duplicate",
		Slug:       "duplicate",
		GitRemote:  "github.com/org/repo",
		Visibility: store.VisibilityPrivate,
	}
	err = s.CreateGrove(ctx, duplicate)
	assert.ErrorIs(t, err, store.ErrAlreadyExists)

	// Update grove
	retrieved.Name = "Updated Project"
	err = s.UpdateGrove(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	retrieved, err = s.GetGrove(ctx, grove.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Project", retrieved.Name)

	// Delete grove
	err = s.DeleteGrove(ctx, grove.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = s.GetGrove(ctx, grove.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestGroveList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a broker for ActiveBrokerCount
	broker := &store.RuntimeBroker{
		ID:     api.NewUUID(),
		Name:   "Test Broker",
		Slug:   "test-broker",
		Status: store.BrokerStatusOnline,
	}
	require.NoError(t, s.CreateRuntimeBroker(ctx, broker))

	// Create groves
	for i := 0; i < 3; i++ {
		grove := &store.Grove{
			ID:         api.NewUUID(),
			Name:       "Grove " + string(rune('A'+i)),
			Slug:       "grove-" + string(rune('a'+i)),
			Visibility: store.VisibilityPrivate,
		}
		if i == 0 {
			grove.Visibility = store.VisibilityPublic
		}
		require.NoError(t, s.CreateGrove(ctx, grove))

		// Add an agent to the first grove
		if i == 0 {
			agent := &store.Agent{
				ID:      api.NewUUID(),
				Slug:    "test-agent",
				Name:    "Test Agent",
				GroveID: grove.ID,
				Status:  store.AgentStatusRunning,
			}
			require.NoError(t, s.CreateAgent(ctx, agent))

			// Link the broker to the first grove
			require.NoError(t, s.AddGroveProvider(ctx, &store.GroveProvider{
				GroveID:    grove.ID,
				BrokerID:   broker.ID,
				BrokerName: broker.Name,
				Status:     store.BrokerStatusOnline,
			}))
		}
	}

	// List all
	result, err := s.ListGroves(ctx, store.GroveFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)

	// Verify computed fields on the first grove (index 2 due to DESC sort by created_at)
	var firstGrove store.Grove
	for _, g := range result.Items {
		if g.Name == "Grove A" {
			firstGrove = g
			break
		}
	}
	assert.Equal(t, 1, firstGrove.AgentCount)
	assert.Equal(t, 1, firstGrove.ActiveBrokerCount)

	// List by visibility
	result, err = s.ListGroves(ctx, store.GroveFilter{Visibility: store.VisibilityPublic}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "Grove A", result.Items[0].Name)
}

// ============================================================================
// RuntimeBroker Tests
// ============================================================================

func TestGroveLookupCaseInsensitive(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a grove with mixed case name
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Global",
		Slug:       "global",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Look up with exact case - should work
	retrieved, err := s.GetGroveBySlugCaseInsensitive(ctx, "global")
	require.NoError(t, err)
	assert.Equal(t, grove.ID, retrieved.ID)

	// Look up with different case - should still work
	retrieved, err = s.GetGroveBySlugCaseInsensitive(ctx, "GLOBAL")
	require.NoError(t, err)
	assert.Equal(t, grove.ID, retrieved.ID)

	// Look up with mixed case - should still work
	retrieved, err = s.GetGroveBySlugCaseInsensitive(ctx, "Global")
	require.NoError(t, err)
	assert.Equal(t, grove.ID, retrieved.ID)

	// Look up non-existent - should return ErrNotFound
	_, err = s.GetGroveBySlugCaseInsensitive(ctx, "nonexistent")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestRuntimeBrokerLookupByName(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a broker
	broker := &store.RuntimeBroker{
		ID:     api.NewUUID(),
		Name:   "My-Laptop",
		Slug:   "my-laptop",
				Status: store.BrokerStatusOnline,
	}
	require.NoError(t, s.CreateRuntimeBroker(ctx, broker))

	// Look up with exact case - should work
	retrieved, err := s.GetRuntimeBrokerByName(ctx, "My-Laptop")
	require.NoError(t, err)
	assert.Equal(t, broker.ID, retrieved.ID)

	// Look up with different case - should still work (case-insensitive)
	retrieved, err = s.GetRuntimeBrokerByName(ctx, "my-laptop")
	require.NoError(t, err)
	assert.Equal(t, broker.ID, retrieved.ID)

	// Look up with all caps - should still work
	retrieved, err = s.GetRuntimeBrokerByName(ctx, "MY-LAPTOP")
	require.NoError(t, err)
	assert.Equal(t, broker.ID, retrieved.ID)

	// Look up non-existent - should return ErrNotFound
	_, err = s.GetRuntimeBrokerByName(ctx, "nonexistent")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// ============================================================================
// RuntimeBroker Tests
// ============================================================================

func TestRuntimeBrokerCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create broker with CreatedBy tracking
	broker := &store.RuntimeBroker{
		ID:      api.NewUUID(),
		Name:    "Dev Laptop",
		Slug:    "dev-laptop",
				Version: "1.0.0",
		Status:  store.BrokerStatusOnline,
		Capabilities: &store.BrokerCapabilities{
			WebPTY: true,
			Sync:   true,
			Attach: true,
		},
		Profiles: []store.BrokerProfile{
			{Name: "default", Type: "docker", Available: true},
		},
		CreatedBy: "admin-user-456",
	}

	err := s.CreateRuntimeBroker(ctx, broker)
	require.NoError(t, err)
	assert.NotZero(t, broker.Created)

	// Get broker
	retrieved, err := s.GetRuntimeBroker(ctx, broker.ID)
	require.NoError(t, err)
	assert.Equal(t, broker.Name, retrieved.Name)
	assert.True(t, retrieved.Capabilities.WebPTY)
	assert.Len(t, retrieved.Profiles, 1)
	assert.Equal(t, "docker", retrieved.Profiles[0].Type)
	assert.Equal(t, "admin-user-456", retrieved.CreatedBy)

	// Update broker
	retrieved.Status = store.BrokerStatusOffline
	err = s.UpdateRuntimeBroker(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	retrieved, err = s.GetRuntimeBroker(ctx, broker.ID)
	require.NoError(t, err)
	assert.Equal(t, store.BrokerStatusOffline, retrieved.Status)

	// Update heartbeat
	err = s.UpdateRuntimeBrokerHeartbeat(ctx, broker.ID, store.BrokerStatusOnline)
	require.NoError(t, err)

	// Verify heartbeat
	retrieved, err = s.GetRuntimeBroker(ctx, broker.ID)
	require.NoError(t, err)
	assert.Equal(t, store.BrokerStatusOnline, retrieved.Status)
	assert.NotZero(t, retrieved.LastHeartbeat)

	// Delete broker
	err = s.DeleteRuntimeBroker(ctx, broker.ID)
	require.NoError(t, err)

	_, err = s.GetRuntimeBroker(ctx, broker.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestRuntimeBrokerList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create brokers
	for i := 0; i < 3; i++ {
		broker := &store.RuntimeBroker{
			ID:     api.NewUUID(),
			Name:   "Host " + string(rune('A'+i)),
			Slug:   "host-" + string(rune('a'+i)),
						Status: store.BrokerStatusOnline,
			Profiles: []store.BrokerProfile{
				{Name: "default", Type: "docker", Available: true},
			},
		}
		if i == 0 {
			broker.Status = store.BrokerStatusOffline
		}
		require.NoError(t, s.CreateRuntimeBroker(ctx, broker))
	}

	// List all
	result, err := s.ListRuntimeBrokers(ctx, store.RuntimeBrokerFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)

	// List by status
	result, err = s.ListRuntimeBrokers(ctx, store.RuntimeBrokerFilter{Status: store.BrokerStatusOffline}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)

	// List by name (exact match, case-insensitive)
	result, err = s.ListRuntimeBrokers(ctx, store.RuntimeBrokerFilter{Name: "Host A"}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "Host A", result.Items[0].Name)

	// List by name (case-insensitive)
	result, err = s.ListRuntimeBrokers(ctx, store.RuntimeBrokerFilter{Name: "host b"}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "Host B", result.Items[0].Name)

	// List by name (no match)
	result, err = s.ListRuntimeBrokers(ctx, store.RuntimeBrokerFilter{Name: "nonexistent"}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalCount)
}

// ============================================================================
// Template Tests
// ============================================================================

func TestTemplateCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create template
	template := &store.Template{
		ID:         api.NewUUID(),
		Name:       "Claude Default",
		Slug:       "claude-default",
		Harness:    "claude",
		Image:      "scion-claude:latest",
		Scope:      "global",
		Visibility: store.VisibilityPublic,
		Config: &store.TemplateConfig{
			Harness:  "claude",
			Detached: true,
		},
	}

	err := s.CreateTemplate(ctx, template)
	require.NoError(t, err)
	assert.NotZero(t, template.Created)

	// Get template
	retrieved, err := s.GetTemplate(ctx, template.ID)
	require.NoError(t, err)
	assert.Equal(t, template.Name, retrieved.Name)
	assert.Equal(t, template.Harness, retrieved.Harness)
	assert.True(t, retrieved.Config.Detached)

	// Get by slug
	retrieved, err = s.GetTemplateBySlug(ctx, "claude-default", "global", "")
	require.NoError(t, err)
	assert.Equal(t, template.ID, retrieved.ID)

	// Update template
	retrieved.Image = "scion-claude:v2"
	err = s.UpdateTemplate(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	retrieved, err = s.GetTemplate(ctx, template.ID)
	require.NoError(t, err)
	assert.Equal(t, "scion-claude:v2", retrieved.Image)

	// Delete template
	err = s.DeleteTemplate(ctx, template.ID)
	require.NoError(t, err)

	_, err = s.GetTemplate(ctx, template.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestTemplateList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create templates
	for i := 0; i < 3; i++ {
		template := &store.Template{
			ID:         api.NewUUID(),
			Name:       "Template " + string(rune('A'+i)),
			Slug:       "template-" + string(rune('a'+i)),
			Harness:    "claude",
			Scope:      "global",
			Visibility: store.VisibilityPublic,
		}
		if i == 0 {
			template.Harness = "gemini"
		}
		require.NoError(t, s.CreateTemplate(ctx, template))
	}

	// List all
	result, err := s.ListTemplates(ctx, store.TemplateFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)

	// List by harness
	result, err = s.ListTemplates(ctx, store.TemplateFilter{Harness: "gemini"}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
}

// ============================================================================
// HarnessConfig Tests
// ============================================================================

func TestHarnessConfigCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create harness config
	hc := &store.HarnessConfig{
		ID:         api.NewUUID(),
		Name:       "Claude Default",
		Slug:       "claude-default",
		Harness:    "claude",
		Scope:      "global",
		Visibility: store.VisibilityPublic,
		Config: &store.HarnessConfigData{
			Harness: "claude",
			Image:   "scion-claude:latest",
		},
	}

	err := s.CreateHarnessConfig(ctx, hc)
	require.NoError(t, err)
	assert.NotZero(t, hc.Created)

	// Get harness config
	retrieved, err := s.GetHarnessConfig(ctx, hc.ID)
	require.NoError(t, err)
	assert.Equal(t, hc.Name, retrieved.Name)
	assert.Equal(t, hc.Harness, retrieved.Harness)
	assert.Equal(t, "claude", retrieved.Config.Harness)
	assert.Equal(t, "scion-claude:latest", retrieved.Config.Image)

	// Get by slug
	retrieved, err = s.GetHarnessConfigBySlug(ctx, "claude-default", "global", "")
	require.NoError(t, err)
	assert.Equal(t, hc.ID, retrieved.ID)

	// Update harness config
	retrieved.Description = "Updated description"
	err = s.UpdateHarnessConfig(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	retrieved, err = s.GetHarnessConfig(ctx, hc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", retrieved.Description)

	// Delete harness config
	err = s.DeleteHarnessConfig(ctx, hc.ID)
	require.NoError(t, err)

	_, err = s.GetHarnessConfig(ctx, hc.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestHarnessConfigList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create harness configs
	for i := 0; i < 3; i++ {
		hc := &store.HarnessConfig{
			ID:         api.NewUUID(),
			Name:       "HC " + string(rune('A'+i)),
			Slug:       "hc-" + string(rune('a'+i)),
			Harness:    "claude",
			Scope:      "global",
			Visibility: store.VisibilityPublic,
		}
		if i == 0 {
			hc.Harness = "gemini"
		}
		require.NoError(t, s.CreateHarnessConfig(ctx, hc))
	}

	// List all
	result, err := s.ListHarnessConfigs(ctx, store.HarnessConfigFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)

	// List by harness
	result, err = s.ListHarnessConfigs(ctx, store.HarnessConfigFilter{Harness: "gemini"}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
}

// ============================================================================
// User Tests
// ============================================================================

func TestUserCRUD(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create user
	user := &store.User{
		ID:          api.NewUUID(),
		Email:       "test@example.com",
		DisplayName: "Test User",
		Role:        store.UserRoleMember,
		Status:      "active",
		Preferences: &store.UserPreferences{
			Theme: "dark",
		},
	}

	err := s.CreateUser(ctx, user)
	require.NoError(t, err)
	assert.NotZero(t, user.Created)

	// Get user
	retrieved, err := s.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, retrieved.Email)
	assert.Equal(t, "dark", retrieved.Preferences.Theme)

	// Get by email
	retrieved, err = s.GetUserByEmail(ctx, "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)

	// Test unique constraint on email
	duplicate := &store.User{
		ID:          api.NewUUID(),
		Email:       "test@example.com",
		DisplayName: "Duplicate User",
		Role:        store.UserRoleMember,
		Status:      "active",
	}
	err = s.CreateUser(ctx, duplicate)
	assert.ErrorIs(t, err, store.ErrAlreadyExists)

	// Update user
	retrieved.DisplayName = "Updated User"
	retrieved.LastLogin = time.Now()
	err = s.UpdateUser(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	retrieved, err = s.GetUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated User", retrieved.DisplayName)
	assert.NotZero(t, retrieved.LastLogin)

	// Delete user
	err = s.DeleteUser(ctx, user.ID)
	require.NoError(t, err)

	_, err = s.GetUser(ctx, user.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestUserList(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create users
	for i := 0; i < 3; i++ {
		user := &store.User{
			ID:          api.NewUUID(),
			Email:       "user" + string(rune('a'+i)) + "@example.com",
			DisplayName: "User " + string(rune('A'+i)),
			Role:        store.UserRoleMember,
			Status:      "active",
		}
		if i == 0 {
			user.Role = store.UserRoleAdmin
		}
		require.NoError(t, s.CreateUser(ctx, user))
	}

	// List all
	result, err := s.ListUsers(ctx, store.UserFilter{}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalCount)

	// List by role
	result, err = s.ListUsers(ctx, store.UserFilter{Role: store.UserRoleAdmin}, store.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalCount)
}

// ============================================================================
// GroveProvider Tests
// ============================================================================

func TestGroveProviders(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create brokers
	broker1 := &store.RuntimeBroker{
		ID:     api.NewUUID(),
		Name:   "Host 1",
		Slug:   "host-1",
				Status: store.BrokerStatusOnline,
		Profiles: []store.BrokerProfile{
			{Name: "docker", Type: "docker", Available: true},
			{Name: "dev", Type: "docker", Available: true},
		},
	}
	require.NoError(t, s.CreateRuntimeBroker(ctx, broker1))

	broker2 := &store.RuntimeBroker{
		ID:     api.NewUUID(),
		Name:   "Host 2",
		Slug:   "host-2",
				Status: store.BrokerStatusOnline,
		Profiles: []store.BrokerProfile{
			{Name: "k8s-prod", Type: "kubernetes", Available: true},
		},
	}
	require.NoError(t, s.CreateRuntimeBroker(ctx, broker2))

	// Add providers with user tracking
	provider1 := &store.GroveProvider{
		GroveID:    grove.ID,
		BrokerID:   broker1.ID,
		BrokerName: broker1.Name,
		Status:     store.BrokerStatusOnline,
		LinkedBy:   "user-123",
	}
	require.NoError(t, s.AddGroveProvider(ctx, provider1))

	provider2 := &store.GroveProvider{
		GroveID:    grove.ID,
		BrokerID:   broker2.ID,
		BrokerName: broker2.Name,
		Status:     store.BrokerStatusOnline,
	}
	require.NoError(t, s.AddGroveProvider(ctx, provider2))

	// Get grove providers
	providers, err := s.GetGroveProviders(ctx, grove.ID)
	require.NoError(t, err)
	assert.Len(t, providers, 2)

	// Verify user tracking fields are stored
	for _, p := range providers {
		if p.BrokerID == broker1.ID {
			assert.Equal(t, "user-123", p.LinkedBy)
			assert.False(t, p.LinkedAt.IsZero(), "LinkedAt should be set")
		}
	}

	// Verify GetGroveProvider also returns user tracking fields
	provider, err := s.GetGroveProvider(ctx, grove.ID, broker1.ID)
	require.NoError(t, err)
	assert.Equal(t, "user-123", provider.LinkedBy)
	assert.False(t, provider.LinkedAt.IsZero(), "LinkedAt should be set")

	// Get broker groves
	groves, err := s.GetBrokerGroves(ctx, broker1.ID)
	require.NoError(t, err)
	assert.Len(t, groves, 1)
	assert.Equal(t, grove.ID, groves[0].GroveID)

	// Update provider status
	err = s.UpdateProviderStatus(ctx, grove.ID, broker1.ID, store.BrokerStatusOffline)
	require.NoError(t, err)

	// Verify update
	providers, err = s.GetGroveProviders(ctx, grove.ID)
	require.NoError(t, err)
	for _, p := range providers {
		if p.BrokerID == broker1.ID {
			assert.Equal(t, store.BrokerStatusOffline, p.Status)
		}
	}

	// Verify grove's active broker count
	retrievedGrove, err := s.GetGrove(ctx, grove.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, retrievedGrove.ActiveBrokerCount) // Only broker2 is online

	// Remove provider
	err = s.RemoveGroveProvider(ctx, grove.ID, broker1.ID)
	require.NoError(t, err)

	providers, err = s.GetGroveProviders(ctx, grove.ID)
	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Equal(t, broker2.ID, providers[0].BrokerID)
}

// ============================================================================
// Migration Tests
// ============================================================================

func TestMigration(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)
	defer s.Close()

	ctx := context.Background()

	// Run migrations
	err = s.Migrate(ctx)
	require.NoError(t, err)

	// Run again (should be idempotent)
	err = s.Migrate(ctx)
	require.NoError(t, err)

	// Verify tables exist by inserting data
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test",
		Slug:       "test",
		Visibility: store.VisibilityPrivate,
	}
	err = s.CreateGrove(ctx, grove)
	require.NoError(t, err)
}

func TestPing(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	err := s.Ping(ctx)
	require.NoError(t, err)
}

// ============================================================================
// Error Cases
// ============================================================================

func TestNotFoundErrors(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	nonExistentID := api.NewUUID()

	// Agent
	_, err := s.GetAgent(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteAgent(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// Grove
	_, err = s.GetGrove(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteGrove(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// RuntimeBroker
	_, err = s.GetRuntimeBroker(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteRuntimeBroker(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// Template
	_, err = s.GetTemplate(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteTemplate(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// User
	_, err = s.GetUser(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteUser(ctx, nonExistentID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestCascadeDelete(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create grove with agent
	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Test Grove",
		Slug:       "test-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	agent := &store.Agent{
		ID:         api.NewUUID(),
		Slug:    "test-agent",
		Name:       "Test Agent",
		Template:   "claude",
		GroveID:    grove.ID,
		Status:     store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, agent))

	// Delete grove
	err := s.DeleteGrove(ctx, grove.ID)
	require.NoError(t, err)

	// Verify agent was cascade deleted
	_, err = s.GetAgent(ctx, agent.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// ============================================================================
// MarkStaleAgentsUndetermined Tests
// ============================================================================

func TestMarkStaleAgentsUndetermined(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Heartbeat Grove",
		Slug:       "heartbeat-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	// Create agents in various states with stale heartbeats (last_seen 5 minutes ago)
	staleTime := time.Now().Add(-5 * time.Minute)
	threshold := time.Now().Add(-2 * time.Minute)

	// These agents should be marked undetermined (active states with stale heartbeat)
	activeStatuses := []string{
		store.AgentStatusRunning,
		store.AgentStatusBusy,
		store.AgentStatusIdle,
		store.AgentStatusWaitingForInput,
		store.AgentStatusProvisioning,
		store.AgentStatusCloning,
	}

	var expectedIDs []string
	for i, status := range activeStatuses {
		agent := &store.Agent{
			ID:         api.NewUUID(),
			Slug:       "active-agent-" + status,
			Name:       "Active Agent " + status,
			Template:   "claude",
			GroveID:    grove.ID,
			Status:     store.AgentStatusPending,
			Visibility: store.VisibilityPrivate,
		}
		require.NoError(t, s.CreateAgent(ctx, agent))

		// Set the agent to the active status and give it a stale last_seen
		err := s.UpdateAgentStatus(ctx, agent.ID, store.AgentStatusUpdate{
			Status: status,
		})
		require.NoError(t, err, "setting status for agent %d", i)

		// Manually set last_seen to stale time
		_, err = s.db.ExecContext(ctx, "UPDATE agents SET last_seen = ? WHERE id = ?", staleTime, agent.ID)
		require.NoError(t, err)

		expectedIDs = append(expectedIDs, agent.ID)
	}

	// These agents should NOT be marked undetermined

	// Terminal state: stopped
	stoppedAgent := &store.Agent{
		ID: api.NewUUID(), Slug: "stopped-agent", Name: "Stopped Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusStopped,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, stoppedAgent))
	_, err := s.db.ExecContext(ctx, "UPDATE agents SET last_seen = ? WHERE id = ?", staleTime, stoppedAgent.ID)
	require.NoError(t, err)

	// Terminal state: completed
	completedAgent := &store.Agent{
		ID: api.NewUUID(), Slug: "completed-agent", Name: "Completed Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusCompleted,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, completedAgent))
	_, err = s.db.ExecContext(ctx, "UPDATE agents SET last_seen = ? WHERE id = ?", staleTime, completedAgent.ID)
	require.NoError(t, err)

	// Terminal state: error
	errorAgent := &store.Agent{
		ID: api.NewUUID(), Slug: "error-agent", Name: "Error Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusError,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, errorAgent))
	_, err = s.db.ExecContext(ctx, "UPDATE agents SET last_seen = ? WHERE id = ?", staleTime, errorAgent.ID)
	require.NoError(t, err)

	// No heartbeat yet (last_seen is NULL)
	noHeartbeatAgent := &store.Agent{
		ID: api.NewUUID(), Slug: "no-heartbeat", Name: "No Heartbeat Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, noHeartbeatAgent))
	// last_seen is NULL by default from CreateAgent (the zero time.Time becomes NULL)

	// Recent heartbeat (should not be affected)
	recentAgent := &store.Agent{
		ID: api.NewUUID(), Slug: "recent-agent", Name: "Recent Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, recentAgent))
	err = s.UpdateAgentStatus(ctx, recentAgent.ID, store.AgentStatusUpdate{
		Status: store.AgentStatusRunning,
	})
	require.NoError(t, err)
	// last_seen is set to now by UpdateAgentStatus, which is within the threshold

	// Execute
	agents, err := s.MarkStaleAgentsUndetermined(ctx, threshold)
	require.NoError(t, err)
	assert.Len(t, agents, len(activeStatuses), "should only mark active stale agents")

	// Verify the returned agents match expected
	returnedIDs := make(map[string]bool)
	for _, a := range agents {
		returnedIDs[a.ID] = true
		assert.Equal(t, store.AgentStatusUndetermined, a.Status, "returned agent should have undetermined status")
	}
	for _, id := range expectedIDs {
		assert.True(t, returnedIDs[id], "expected agent %s to be in returned set", id)
	}

	// Verify agents in DB are actually updated
	for _, id := range expectedIDs {
		a, err := s.GetAgent(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, store.AgentStatusUndetermined, a.Status, "DB agent should have undetermined status")
	}

	// Verify non-target agents were NOT affected
	a, err := s.GetAgent(ctx, stoppedAgent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusStopped, a.Status)

	a, err = s.GetAgent(ctx, completedAgent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusCompleted, a.Status)

	a, err = s.GetAgent(ctx, recentAgent.ID)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusRunning, a.Status)
}

func TestMarkStaleAgentsUndetermined_Idempotent(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	grove := &store.Grove{
		ID:         api.NewUUID(),
		Name:       "Idempotent Grove",
		Slug:       "idempotent-grove",
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateGrove(ctx, grove))

	staleTime := time.Now().Add(-5 * time.Minute)
	threshold := time.Now().Add(-2 * time.Minute)

	agent := &store.Agent{
		ID: api.NewUUID(), Slug: "stale-agent", Name: "Stale Agent",
		Template: "claude", GroveID: grove.ID, Status: store.AgentStatusRunning,
		Visibility: store.VisibilityPrivate,
	}
	require.NoError(t, s.CreateAgent(ctx, agent))
	_, err := s.db.ExecContext(ctx, "UPDATE agents SET last_seen = ? WHERE id = ?", staleTime, agent.ID)
	require.NoError(t, err)

	// First call should mark it undetermined
	agents, err := s.MarkStaleAgentsUndetermined(ctx, threshold)
	require.NoError(t, err)
	assert.Len(t, agents, 1)

	// Second call should return empty (already undetermined)
	agents, err = s.MarkStaleAgentsUndetermined(ctx, threshold)
	require.NoError(t, err)
	assert.Len(t, agents, 0, "should not re-mark already undetermined agents")
}

func TestMarkStaleAgentsUndetermined_NoStaleAgents(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	threshold := time.Now().Add(-2 * time.Minute)

	// No agents at all
	agents, err := s.MarkStaleAgentsUndetermined(ctx, threshold)
	require.NoError(t, err)
	assert.Len(t, agents, 0)
}
