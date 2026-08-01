package memory

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
)

func TestMemoryStorageInstanceStateContract(t *testing.T) {
	ctx := context.Background()
	storage := NewStateStorage()
	createdAt := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	finishedAt := createdAt.Add(2 * time.Minute)

	states := []glowflow.InstanceState{
		{
			InstanceID:     "inst-003",
			ChainID:        "flow-b",
			Version:        "v1",
			Status:         glowflow.InstanceStatusFailed,
			CurrentNodeIDs: []string{"archive"},
			Variables:      map[string]any{"attempts": float64(2)},
			Error:          "boom",
			CreatedAt:      createdAt.Add(3 * time.Minute),
			UpdatedAt:      updatedAt.Add(3 * time.Minute),
			FinishedAt:     &finishedAt,
		},
		{
			InstanceID:     "inst-001",
			ChainID:        "flow-a",
			Version:        "v1",
			Status:         glowflow.InstanceStatusRunning,
			CurrentNodeIDs: []string{"node-a", "node-b"},
			Variables:      map[string]any{"count": float64(1), "mode": "fast"},
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		},
		{
			InstanceID: "inst-002",
			ChainID:    "flow-a",
			Version:    "v2",
			Status:     glowflow.InstanceStatusCompleted,
			Variables:  map[string]any{"count": float64(2)},
			CreatedAt:  createdAt.Add(time.Minute),
			UpdatedAt:  updatedAt.Add(time.Minute),
			FinishedAt: &finishedAt,
		},
		{
			InstanceID: "inst-004",
			ChainID:    "flow-a",
			Version:    "v1",
			Status:     glowflow.InstanceStatusPaused,
			CreatedAt:  createdAt.Add(4 * time.Minute),
			UpdatedAt:  updatedAt.Add(4 * time.Minute),
		},
	}

	for _, state := range states {
		if err := storage.SaveInstanceState(ctx, state); err != nil {
			t.Fatalf("save instance state %q: %v", state.InstanceID, err)
		}
	}

	got, err := storage.GetInstanceState(ctx, "inst-001")
	if err != nil {
		t.Fatalf("get instance state: %v", err)
	}
	if !reflect.DeepEqual(got, states[1]) {
		t.Fatalf("instance state = %#v, want %#v", got, states[1])
	}

	list, err := storage.ListInstanceStates(ctx, glowflow.InstanceStateFilter{
		ChainID:  "flow-a",
		Version:  "v1",
		Statuses: []glowflow.InstanceStatus{glowflow.InstanceStatusPaused, glowflow.InstanceStatusRunning},
		Limit:    1,
		Offset:   1,
	})
	if err != nil {
		t.Fatalf("list instance states: %v", err)
	}
	if gotIDs := instanceStateIDs(list); !reflect.DeepEqual(gotIDs, []string{"inst-004"}) {
		t.Fatalf("instance list IDs = %#v, want deterministic offset order", gotIDs)
	}

	if err := storage.DeleteInstanceState(ctx, "inst-001"); err != nil {
		t.Fatalf("delete instance state: %v", err)
	}
	if _, err := storage.GetInstanceState(ctx, "inst-001"); !errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("deleted instance state error = %v, want ErrStateNotFound", err)
	}
	if _, err := storage.GetInstanceState(ctx, "inst-missing"); !errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("missing instance state error = %v, want ErrStateNotFound", err)
	}
}

func TestMemoryStorageNodeStateContract(t *testing.T) {
	ctx := context.Background()
	storage := NewStateStorage()
	startedAt := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)

	states := []glowflow.NodeState{
		{
			InstanceID: "inst-001",
			ChainID:    "flow-a",
			Version:    "v1",
			NodeID:     "node-c",
			Status:     glowflow.NodeStatusSkipped,
			Attempts:   0,
			Metadata:   map[string]any{"reason": "branch"},
		},
		{
			InstanceID: "inst-001",
			ChainID:    "flow-a",
			Version:    "v1",
			NodeID:     "node-a",
			Status:     glowflow.NodeStatusRunning,
			Attempts:   1,
			Input:      map[string]any{"payload": "in"},
			Output:     map[string]any{"payload": "out"},
			Metadata:   map[string]any{"worker": "w1"},
			StartedAt:  &startedAt,
		},
		{
			InstanceID: "inst-001",
			ChainID:    "flow-a",
			Version:    "v2",
			NodeID:     "node-b",
			Status:     glowflow.NodeStatusCompleted,
			Attempts:   2,
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		},
		{
			InstanceID: "inst-002",
			ChainID:    "flow-b",
			Version:    "v1",
			NodeID:     "node-a",
			Status:     glowflow.NodeStatusFailed,
			Attempts:   3,
			Error:      "boom",
		},
	}

	for _, state := range states {
		if err := storage.SaveNodeState(ctx, state); err != nil {
			t.Fatalf("save node state %q/%q: %v", state.InstanceID, state.NodeID, err)
		}
	}

	got, err := storage.GetNodeState(ctx, "inst-001", "node-a")
	if err != nil {
		t.Fatalf("get node state: %v", err)
	}
	if !reflect.DeepEqual(got, states[1]) {
		t.Fatalf("node state = %#v, want %#v", got, states[1])
	}

	list, err := storage.ListNodeStates(ctx, glowflow.NodeStateFilter{
		InstanceID: "inst-001",
		ChainID:    "flow-a",
		NodeID:     "node",
		Statuses:   []glowflow.NodeStatus{glowflow.NodeStatusCompleted, glowflow.NodeStatusRunning, glowflow.NodeStatusSkipped},
		Limit:      2,
		Offset:     1,
	})
	if err != nil {
		t.Fatalf("list node states: %v", err)
	}
	if gotIDs := nodeStateIDs(list); !reflect.DeepEqual(gotIDs, []string{"inst-001/node-b", "inst-001/node-c"}) {
		t.Fatalf("node list IDs = %#v, want deterministic offset order", gotIDs)
	}

	if err := storage.DeleteNodeState(ctx, "inst-001", "node-a"); err != nil {
		t.Fatalf("delete node state: %v", err)
	}
	if _, err := storage.GetNodeState(ctx, "inst-001", "node-a"); !errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("deleted node state error = %v, want ErrStateNotFound", err)
	}
	if _, err := storage.GetNodeState(ctx, "inst-missing", "node-missing"); !errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("missing node state error = %v, want ErrStateNotFound", err)
	}
}

func TestMemoryStorageStateCopies(t *testing.T) {
	ctx := context.Background()
	storage := NewStateStorage()
	finishedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	wantFinishedAt := finishedAt
	state := glowflow.InstanceState{
		InstanceID:     "inst-copy",
		ChainID:        "flow-copy",
		Status:         glowflow.InstanceStatusRunning,
		CurrentNodeIDs: []string{"node-a"},
		Variables: map[string]any{
			"count":  float64(1),
			"nested": map[string]any{"value": "kept"},
			"items":  []any{"a", "b"},
		},
		FinishedAt: &finishedAt,
	}

	if err := storage.SaveInstanceState(ctx, state); err != nil {
		t.Fatalf("save instance state: %v", err)
	}
	state.CurrentNodeIDs[0] = "changed"
	state.Variables["count"] = float64(99)
	state.Variables["nested"].(map[string]any)["value"] = "changed"
	state.Variables["items"].([]any)[0] = "changed"
	*state.FinishedAt = state.FinishedAt.Add(time.Hour)

	got, err := storage.GetInstanceState(ctx, "inst-copy")
	if err != nil {
		t.Fatalf("get instance state: %v", err)
	}
	if got.CurrentNodeIDs[0] != "node-a" || got.Variables["count"] != float64(1) || got.Variables["nested"].(map[string]any)["value"] != "kept" || got.Variables["items"].([]any)[0] != "a" || !got.FinishedAt.Equal(wantFinishedAt) {
		t.Fatalf("stored instance state was mutated through original value: %#v", got)
	}

	got.CurrentNodeIDs[0] = "changed-again"
	got.Variables["count"] = float64(100)
	got.Variables["nested"].(map[string]any)["value"] = "changed-again"
	got.Variables["items"].([]any)[0] = "changed-again"
	*got.FinishedAt = got.FinishedAt.Add(time.Hour)
	again, err := storage.GetInstanceState(ctx, "inst-copy")
	if err != nil {
		t.Fatalf("get instance state again: %v", err)
	}
	if again.CurrentNodeIDs[0] != "node-a" || again.Variables["count"] != float64(1) || again.Variables["nested"].(map[string]any)["value"] != "kept" || again.Variables["items"].([]any)[0] != "a" || !again.FinishedAt.Equal(wantFinishedAt) {
		t.Fatalf("stored instance state was mutated through returned value: %#v", again)
	}

	startedAt := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)
	wantStartedAt := startedAt
	nodeState := glowflow.NodeState{
		InstanceID: "inst-copy",
		ChainID:    "flow-copy",
		NodeID:     "node-copy",
		Status:     glowflow.NodeStatusRunning,
		Input:      map[string]any{"payload": "in"},
		Output:     map[string]any{"payload": "out"},
		Metadata: map[string]any{
			"worker": "w1",
			"nested": map[string]any{"value": "kept"},
			"items":  []any{"a", "b"},
		},
		StartedAt: &startedAt,
	}

	if err := storage.SaveNodeState(ctx, nodeState); err != nil {
		t.Fatalf("save node state: %v", err)
	}
	nodeState.Input.(map[string]any)["payload"] = "changed"
	nodeState.Output.(map[string]any)["payload"] = "changed"
	nodeState.Metadata["worker"] = "changed"
	nodeState.Metadata["nested"].(map[string]any)["value"] = "changed"
	nodeState.Metadata["items"].([]any)[0] = "changed"
	*nodeState.StartedAt = nodeState.StartedAt.Add(time.Hour)

	gotNode, err := storage.GetNodeState(ctx, "inst-copy", "node-copy")
	if err != nil {
		t.Fatalf("get node state: %v", err)
	}
	if gotNode.Input.(map[string]any)["payload"] != "in" || gotNode.Output.(map[string]any)["payload"] != "out" || gotNode.Metadata["worker"] != "w1" || gotNode.Metadata["nested"].(map[string]any)["value"] != "kept" || gotNode.Metadata["items"].([]any)[0] != "a" || !gotNode.StartedAt.Equal(wantStartedAt) {
		t.Fatalf("stored node state was mutated through original value: %#v", gotNode)
	}

	gotNode.Input.(map[string]any)["payload"] = "changed-again"
	gotNode.Output.(map[string]any)["payload"] = "changed-again"
	gotNode.Metadata["worker"] = "changed-again"
	gotNode.Metadata["nested"].(map[string]any)["value"] = "changed-again"
	gotNode.Metadata["items"].([]any)[0] = "changed-again"
	*gotNode.StartedAt = gotNode.StartedAt.Add(time.Hour)
	againNode, err := storage.GetNodeState(ctx, "inst-copy", "node-copy")
	if err != nil {
		t.Fatalf("get node state again: %v", err)
	}
	if againNode.Input.(map[string]any)["payload"] != "in" || againNode.Output.(map[string]any)["payload"] != "out" || againNode.Metadata["worker"] != "w1" || againNode.Metadata["nested"].(map[string]any)["value"] != "kept" || againNode.Metadata["items"].([]any)[0] != "a" || !againNode.StartedAt.Equal(wantStartedAt) {
		t.Fatalf("stored node state was mutated through returned value: %#v", againNode)
	}
}

func instanceStateIDs(states []glowflow.InstanceState) []string {
	ids := make([]string, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.InstanceID)
	}
	return ids
}

func nodeStateIDs(states []glowflow.NodeState) []string {
	ids := make([]string, 0, len(states))
	for _, state := range states {
		ids = append(ids, state.InstanceID+"/"+state.NodeID)
	}
	return ids
}
