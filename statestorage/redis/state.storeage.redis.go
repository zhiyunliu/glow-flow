package redis

import (
	"context"
	_ "embed"
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

//go:embed save_instance_state.lua
var redisSaveInstanceLua string

//go:embed delete_instance_state.lua
var redisDeleteInstanceLua string

//go:embed save_node_state.lua
var redisSaveNodeLua string

//go:embed delete_node_state.lua
var redisDeleteNodeLua string

var (
	redisSaveInstanceScript   = redis.NewScript(redisSaveInstanceLua)
	redisDeleteInstanceScript = redis.NewScript(redisDeleteInstanceLua)
	redisSaveNodeScript       = redis.NewScript(redisSaveNodeLua)
	redisDeleteNodeScript     = redis.NewScript(redisDeleteNodeLua)
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
	key := s.instanceStateKey(state.InstanceID)
	_, err = s.runRedisScript(ctx, redisSaveInstanceScript, []string{key, redisInstanceIndexKey}, data, state.InstanceID)
	return err
}

func (s *redisStorage) GetInstanceState(ctx context.Context, instanceID string) (glowflow.InstanceState, error) {
	data, err := s.redis.Get(ctx, s.instanceStateKey(instanceID)).Bytes()
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

	statuses := s.instanceStatusSet(filter.Statuses)
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
	return s.paginateInstances(states, filter.Offset, filter.Limit), nil
}

func (s *redisStorage) DeleteInstanceState(ctx context.Context, instanceID string) error {
	_, err := s.runRedisScript(ctx, redisDeleteInstanceScript, []string{s.instanceStateKey(instanceID), redisInstanceIndexKey}, instanceID)
	return err
}

func (s *redisStorage) SaveNodeState(ctx context.Context, state glowflow.NodeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	id := s.nodeStateID(state.InstanceID, state.NodeID)
	_, err = s.runRedisScript(ctx, redisSaveNodeScript, []string{s.nodeStateKey(state.InstanceID, state.NodeID), redisNodeIndexKey}, data, id)
	return err
}

func (s *redisStorage) GetNodeState(ctx context.Context, instanceID string, nodeID string) (glowflow.NodeState, error) {
	data, err := s.redis.Get(ctx, s.nodeStateKey(instanceID, nodeID)).Bytes()
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

	statuses := s.nodeStatusSet(filter.Statuses)
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
	return s.paginateNodes(states, filter.Offset, filter.Limit), nil
}

func (s *redisStorage) DeleteNodeState(ctx context.Context, instanceID string, nodeID string) error {
	_, err := s.runRedisScript(ctx, redisDeleteNodeScript, []string{s.nodeStateKey(instanceID, nodeID), redisNodeIndexKey}, s.nodeStateID(instanceID, nodeID))
	return err
}

func (s *redisStorage) runRedisScript(ctx context.Context, script *redis.Script, keys []string, args ...any) (any, error) {
	cmd := script.EvalSha(ctx, s.redis, keys, args...)
	if !redis.HasErrorPrefix(cmd.Err(), "NOSCRIPT") {
		return cmd.Result()
	}
	if err := script.Load(ctx, s.redis).Err(); err != nil {
		return nil, err
	}
	return script.EvalSha(ctx, s.redis, keys, args...).Result()
}

func (s *redisStorage) instanceStateKey(instanceID string) string {
	return "glowflow:state:instance:" + instanceID
}

func (s *redisStorage) nodeStateKey(instanceID string, nodeID string) string {
	return "glowflow:state:node:" + s.nodeStateID(instanceID, nodeID)
}

func (s *redisStorage) nodeStateID(instanceID string, nodeID string) string {
	return instanceID + "/" + nodeID
}

func (s *redisStorage) instanceStatusSet(statuses []glowflow.InstanceStatus) map[glowflow.InstanceStatus]bool {
	set := make(map[glowflow.InstanceStatus]bool, len(statuses))
	for _, status := range statuses {
		set[status] = true
	}
	return set
}

func (s *redisStorage) nodeStatusSet(statuses []glowflow.NodeStatus) map[glowflow.NodeStatus]bool {
	set := make(map[glowflow.NodeStatus]bool, len(statuses))
	for _, status := range statuses {
		set[status] = true
	}
	return set
}

func (s *redisStorage) paginateInstances(states []glowflow.InstanceState, offset int, limit int) []glowflow.InstanceState {
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

func (s *redisStorage) paginateNodes(states []glowflow.NodeState, offset int, limit int) []glowflow.NodeState {
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
