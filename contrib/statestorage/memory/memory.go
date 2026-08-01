package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	glowflow "github.com/zhiyunliu/glow-flow"
)

func NewStateStorage() glowflow.StateStorage {
	return &memoryStateStorage{
		instances: make(map[string]glowflow.InstanceState),
		nodes:     make(map[memoryNodeKey]glowflow.NodeState),
	}
}

type memoryStateStorage struct {
	mu        sync.RWMutex
	instances map[string]glowflow.InstanceState
	nodes     map[memoryNodeKey]glowflow.NodeState
}

type memoryNodeKey struct {
	instanceID string
	nodeID     string
}

func (s *memoryStateStorage) SaveInstanceState(ctx context.Context, state glowflow.InstanceState) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[state.InstanceID] = cloneInstanceState(state)
	return nil
}

func (s *memoryStateStorage) GetInstanceState(ctx context.Context, instanceID string) (glowflow.InstanceState, error) {
	if err := ctx.Err(); err != nil {
		return glowflow.InstanceState{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.instances[instanceID]
	if !ok {
		return glowflow.InstanceState{}, fmt.Errorf("instance state %q: %w", instanceID, glowflow.ErrStateNotFound)
	}
	return cloneInstanceState(state), nil
}

func (s *memoryStateStorage) ListInstanceStates(ctx context.Context, filter glowflow.InstanceStateFilter) ([]glowflow.InstanceState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.instances))
	for id := range s.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	statuses := instanceStatusSet(filter.Statuses)
	states := make([]glowflow.InstanceState, 0, len(ids))
	for _, id := range ids {
		state := s.instances[id]
		if filter.ChainID != "" && state.ChainID != filter.ChainID {
			continue
		}
		if filter.Version != "" && state.Version != filter.Version {
			continue
		}
		if len(statuses) > 0 && !statuses[state.Status] {
			continue
		}
		states = append(states, cloneInstanceState(state))
	}
	return paginateInstances(states, filter.Offset, filter.Limit), nil
}

func (s *memoryStateStorage) DeleteInstanceState(ctx context.Context, instanceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.instances, instanceID)
	return nil
}

func (s *memoryStateStorage) SaveNodeState(ctx context.Context, state glowflow.NodeState) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[memoryNodeKey{instanceID: state.InstanceID, nodeID: state.NodeID}] = cloneNodeState(state)
	return nil
}

func (s *memoryStateStorage) GetNodeState(ctx context.Context, instanceID string, nodeID string) (glowflow.NodeState, error) {
	if err := ctx.Err(); err != nil {
		return glowflow.NodeState{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.nodes[memoryNodeKey{instanceID: instanceID, nodeID: nodeID}]
	if !ok {
		return glowflow.NodeState{}, fmt.Errorf("node state %q/%q: %w", instanceID, nodeID, glowflow.ErrStateNotFound)
	}
	return cloneNodeState(state), nil
}

func (s *memoryStateStorage) ListNodeStates(ctx context.Context, filter glowflow.NodeStateFilter) ([]glowflow.NodeState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]memoryNodeKey, 0, len(s.nodes))
	for key := range s.nodes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		return nodeStateID(keys[i]) < nodeStateID(keys[j])
	})

	statuses := nodeStatusSet(filter.Statuses)
	states := make([]glowflow.NodeState, 0, len(keys))
	for _, key := range keys {
		state := s.nodes[key]
		if filter.InstanceID != "" && state.InstanceID != filter.InstanceID {
			continue
		}
		if filter.ChainID != "" && state.ChainID != filter.ChainID {
			continue
		}
		if filter.NodeID != "" && !strings.Contains(state.NodeID, filter.NodeID) {
			continue
		}
		if len(statuses) > 0 && !statuses[state.Status] {
			continue
		}
		states = append(states, cloneNodeState(state))
	}
	return paginateNodes(states, filter.Offset, filter.Limit), nil
}

func (s *memoryStateStorage) DeleteNodeState(ctx context.Context, instanceID string, nodeID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, memoryNodeKey{instanceID: instanceID, nodeID: nodeID})
	return nil
}

func nodeStateID(key memoryNodeKey) string {
	return key.instanceID + "/" + key.nodeID
}

func instanceStatusSet(statuses []glowflow.InstanceStatus) map[glowflow.InstanceStatus]bool {
	set := make(map[glowflow.InstanceStatus]bool, len(statuses))
	for _, status := range statuses {
		set[status] = true
	}
	return set
}

func nodeStatusSet(statuses []glowflow.NodeStatus) map[glowflow.NodeStatus]bool {
	set := make(map[glowflow.NodeStatus]bool, len(statuses))
	for _, status := range statuses {
		set[status] = true
	}
	return set
}

func paginateInstances(states []glowflow.InstanceState, offset int, limit int) []glowflow.InstanceState {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(states) {
		return []glowflow.InstanceState{}
	}
	states = states[offset:]
	if limit > 0 && limit < len(states) {
		states = states[:limit]
	}
	return states
}

func paginateNodes(states []glowflow.NodeState, offset int, limit int) []glowflow.NodeState {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(states) {
		return []glowflow.NodeState{}
	}
	states = states[offset:]
	if limit > 0 && limit < len(states) {
		states = states[:limit]
	}
	return states
}

func cloneInstanceState(state glowflow.InstanceState) glowflow.InstanceState {
	state.CurrentNodeIDs = append([]string(nil), state.CurrentNodeIDs...)
	state.Variables = cloneMap(state.Variables)
	if state.FinishedAt != nil {
		finishedAt := *state.FinishedAt
		state.FinishedAt = &finishedAt
	}
	return state
}

func cloneNodeState(state glowflow.NodeState) glowflow.NodeState {
	state.Input = cloneAny(state.Input)
	state.Output = cloneAny(state.Output)
	state.Metadata = cloneMap(state.Metadata)
	if state.StartedAt != nil {
		startedAt := *state.StartedAt
		state.StartedAt = &startedAt
	}
	if state.FinishedAt != nil {
		finishedAt := *state.FinishedAt
		state.FinishedAt = &finishedAt
	}
	return state
}

func cloneAny(value any) any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return value
	}
	return cloned
}

func cloneMap[M ~map[string]any](source M) M {
	if source == nil {
		return nil
	}
	clone := make(M, len(source))
	for key, value := range source {
		clone[key] = cloneAny(value)
	}
	return clone
}
