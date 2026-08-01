package defaults

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	redis "github.com/redis/go-redis/v9"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const (
	redisInstanceIndexKey = "glowflow:state:instances"
	redisNodeIndexKey     = "glowflow:state:nodes"
)

type redisStorage struct {
	redis *redis.Client
}

func NewRedisStorage(client *redis.Client) glowflow.StateStorage {
	return &redisStorage{redis: client}
}

func (s *redisStorage) SaveInstanceState(ctx context.Context, state glowflow.InstanceState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	key := instanceStateKey(state.InstanceID)
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, redisInstanceIndexKey, state.InstanceID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisStorage) GetInstanceState(ctx context.Context, instanceID string) (glowflow.InstanceState, error) {
	data, err := s.redis.Get(ctx, instanceStateKey(instanceID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return glowflow.InstanceState{}, fmt.Errorf("instance state %q: %w", instanceID, glowflow.ErrStateNotFound)
		}
		return glowflow.InstanceState{}, err
	}

	var state glowflow.InstanceState
	if err := json.Unmarshal(data, &state); err != nil {
		return glowflow.InstanceState{}, err
	}
	return state, nil
}

func (s *redisStorage) ListInstanceStates(ctx context.Context, filter glowflow.InstanceStateFilter) ([]glowflow.InstanceState, error) {
	ids, err := s.redis.SMembers(ctx, redisInstanceIndexKey).Result()
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)

	statuses := instanceStatusSet(filter.Statuses)
	states := make([]glowflow.InstanceState, 0, len(ids))
	for _, id := range ids {
		state, err := s.GetInstanceState(ctx, id)
		if err != nil {
			if errors.Is(err, glowflow.ErrStateNotFound) {
				continue
			}
			return nil, err
		}
		if filter.ChainID != "" && state.ChainID != filter.ChainID {
			continue
		}
		if filter.Version != "" && state.Version != filter.Version {
			continue
		}
		if len(statuses) > 0 && !statuses[state.Status] {
			continue
		}
		states = append(states, state)
	}
	return paginateInstances(states, filter.Offset, filter.Limit), nil
}

func (s *redisStorage) DeleteInstanceState(ctx context.Context, instanceID string) error {
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, instanceStateKey(instanceID))
	pipe.SRem(ctx, redisInstanceIndexKey, instanceID)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStorage) SaveNodeState(ctx context.Context, state glowflow.NodeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	id := nodeStateID(state.InstanceID, state.NodeID)
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, nodeStateKey(state.InstanceID, state.NodeID), data, 0)
	pipe.SAdd(ctx, redisNodeIndexKey, id)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisStorage) GetNodeState(ctx context.Context, instanceID string, nodeID string) (glowflow.NodeState, error) {
	data, err := s.redis.Get(ctx, nodeStateKey(instanceID, nodeID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return glowflow.NodeState{}, fmt.Errorf("node state %q/%q: %w", instanceID, nodeID, glowflow.ErrStateNotFound)
		}
		return glowflow.NodeState{}, err
	}

	var state glowflow.NodeState
	if err := json.Unmarshal(data, &state); err != nil {
		return glowflow.NodeState{}, err
	}
	return state, nil
}

func (s *redisStorage) ListNodeStates(ctx context.Context, filter glowflow.NodeStateFilter) ([]glowflow.NodeState, error) {
	ids, err := s.redis.SMembers(ctx, redisNodeIndexKey).Result()
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)

	statuses := nodeStatusSet(filter.Statuses)
	states := make([]glowflow.NodeState, 0, len(ids))
	for _, id := range ids {
		instanceID, nodeID, ok := strings.Cut(id, "/")
		if !ok {
			continue
		}
		state, err := s.GetNodeState(ctx, instanceID, nodeID)
		if err != nil {
			if errors.Is(err, glowflow.ErrStateNotFound) {
				continue
			}
			return nil, err
		}
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
		states = append(states, state)
	}
	return paginateNodes(states, filter.Offset, filter.Limit), nil
}

func (s *redisStorage) DeleteNodeState(ctx context.Context, instanceID string, nodeID string) error {
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, nodeStateKey(instanceID, nodeID))
	pipe.SRem(ctx, redisNodeIndexKey, nodeStateID(instanceID, nodeID))
	_, err := pipe.Exec(ctx)
	return err
}

func instanceStateKey(instanceID string) string {
	return "glowflow:state:instance:" + instanceID
}

func nodeStateKey(instanceID string, nodeID string) string {
	return "glowflow:state:node:" + nodeStateID(instanceID, nodeID)
}

func nodeStateID(instanceID string, nodeID string) string {
	return instanceID + "/" + nodeID
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
