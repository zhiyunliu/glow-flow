package redis

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	glowflow "github.com/zhiyunliu/glow-flow"
)

func TestRedisStorageInstanceStateContract(t *testing.T) {
	ctx := context.Background()
	storage := newTestRedisStorage(t)
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

func TestRedisStorageInstanceStateUsesLuaAndReloadsMissingScript(t *testing.T) {
	ctx := context.Background()
	storage, client, commands := newObservedTestRedisStorage(t)
	state := glowflow.InstanceState{
		InstanceID: "inst-lua",
		ChainID:    "flow-lua",
		Version:    "v1",
		Status:     glowflow.InstanceStatusRunning,
		CreatedAt:  time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 8, 1, 12, 1, 0, 0, time.UTC),
	}

	commands.Reset()
	if err := storage.SaveInstanceState(ctx, state); err != nil {
		t.Fatalf("save instance state with lua script: %v", err)
	}
	commands.RequireLuaExecution(t, "initial instance save")

	if err := client.ScriptFlush(ctx).Err(); err != nil {
		t.Fatalf("flush script cache: %v", err)
	}

	commands.Reset()
	state.Status = glowflow.InstanceStatusCompleted
	state.UpdatedAt = state.UpdatedAt.Add(time.Minute)
	if err := storage.SaveInstanceState(ctx, state); err != nil {
		t.Fatalf("save instance state after script flush: %v", err)
	}
	commands.RequireScriptReload(t, "instance save after script flush")

	commands.Reset()
	if err := storage.DeleteInstanceState(ctx, state.InstanceID); err != nil {
		t.Fatalf("delete instance state after script flush: %v", err)
	}
	commands.RequireLuaExecution(t, "instance delete")
}

func TestRedisStorageNodeStateContract(t *testing.T) {
	ctx := context.Background()
	storage := newTestRedisStorage(t)
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

func TestRedisStorageNodeStateUsesLuaAndReloadsMissingScript(t *testing.T) {
	ctx := context.Background()
	storage, client, commands := newObservedTestRedisStorage(t)
	state := glowflow.NodeState{
		InstanceID: "inst-lua",
		ChainID:    "flow-lua",
		Version:    "v1",
		NodeID:     "node-lua",
		Status:     glowflow.NodeStatusRunning,
		Attempts:   1,
		Input:      map[string]any{"payload": "in"},
	}

	commands.Reset()
	if err := storage.SaveNodeState(ctx, state); err != nil {
		t.Fatalf("save node state with lua script: %v", err)
	}
	commands.RequireLuaExecution(t, "initial node save")

	if err := client.ScriptFlush(ctx).Err(); err != nil {
		t.Fatalf("flush script cache: %v", err)
	}

	commands.Reset()
	state.Status = glowflow.NodeStatusCompleted
	state.Output = map[string]any{"payload": "out"}
	if err := storage.SaveNodeState(ctx, state); err != nil {
		t.Fatalf("save node state after script flush: %v", err)
	}
	commands.RequireScriptReload(t, "node save after script flush")

	commands.Reset()
	if err := storage.DeleteNodeState(ctx, state.InstanceID, state.NodeID); err != nil {
		t.Fatalf("delete node state after script flush: %v", err)
	}
	commands.RequireLuaExecution(t, "node delete")
}

func newTestRedisStorage(t *testing.T) glowflow.StateStorage {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	return NewRedisStorage(client)
}

func newObservedTestRedisStorage(t *testing.T) (glowflow.StateStorage, *redis.Client, *redisCommandLog) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	commands := &redisCommandLog{}
	client.AddHook(commands)
	t.Cleanup(func() {
		_ = client.Close()
	})

	return NewRedisStorage(client), client, commands
}

type redisCommandLog struct {
	mu       sync.Mutex
	commands []string
}

func (l *redisCommandLog) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (l *redisCommandLog) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		l.record(cmd.Name())
		return next(ctx, cmd)
	}
}

func (l *redisCommandLog) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			l.record(cmd.Name())
		}
		return next(ctx, cmds)
	}
}

func (l *redisCommandLog) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.commands = nil
}

func (l *redisCommandLog) RequireLuaExecution(t *testing.T, operation string) {
	t.Helper()

	if !l.hasAny("eval", "evalsha", "script") {
		t.Fatalf("%s commands = %#v, want lua script execution", operation, l.snapshot())
	}
}

func (l *redisCommandLog) RequireScriptReload(t *testing.T, operation string) {
	t.Helper()

	if !l.has("evalsha") || !l.hasAny("eval", "script") {
		t.Fatalf("%s commands = %#v, want evalsha plus eval fallback or script reload", operation, l.snapshot())
	}
}

func (l *redisCommandLog) record(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.commands = append(l.commands, strings.ToLower(name))
}

func (l *redisCommandLog) has(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, command := range l.commands {
		if command == name {
			return true
		}
	}
	return false
}

func (l *redisCommandLog) hasAny(names ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, command := range l.commands {
		for _, name := range names {
			if command == name {
				return true
			}
		}
	}
	return false
}

func (l *redisCommandLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.commands...)
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
