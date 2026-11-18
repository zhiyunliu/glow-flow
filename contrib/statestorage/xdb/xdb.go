package xdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/daos"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/models"
	"github.com/zhiyunliu/glue"
	gluexdb "github.com/zhiyunliu/glue/xdb"
)

// NewStateStorage 创建基于 glue xdb 的状态存储，dbConnName 为 glue 配置中的数据库连接名。
//
// 完整的状态以 JSON 形式保存在 state_data 列中，保证读写无损；
// chain_id/version/status 等列冗余保存，仅用于条件查询。
func NewStateStorage(dbConnName string) glowflow.StateStorage {
	return &xdbStorage{dbConnName: dbConnName}
}

type xdbStorage struct {
	dbConnName string
}

var dbResolver = func(name string) gluexdb.Executer {
	return glue.DB(name)
}

func (s *xdbStorage) SaveInstanceState(ctx context.Context, state glowflow.InstanceState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return daos.SaveInstanceState(ctx, dbResolver(s.dbConnName), &models.InstanceState{
		InstanceID: state.InstanceID,
		ChainID:    state.ChainID,
		Version:    state.Version,
		Status:     string(state.Status),
		StateData:  string(data),
	})
}

func (s *xdbStorage) GetInstanceState(ctx context.Context, instanceID string) (glowflow.InstanceState, error) {
	row, err := daos.GetInstanceState(ctx, dbResolver(s.dbConnName), instanceID)
	if err != nil {
		if errors.Is(err, gluexdb.ErrEmptyError) {
			return glowflow.InstanceState{}, fmt.Errorf("instance state %q: %w", instanceID, glowflow.ErrStateNotFound)
		}
		return glowflow.InstanceState{}, err
	}
	return decodeInstanceState(row)
}

func (s *xdbStorage) ListInstanceStates(ctx context.Context, filter glowflow.InstanceStateFilter) ([]glowflow.InstanceState, error) {
	rows, err := daos.ListInstanceStates(ctx, dbResolver(s.dbConnName), daos.InstanceStateFilter{
		ChainID:  filter.ChainID,
		Version:  filter.Version,
		Statuses: instanceStatusStrings(filter.Statuses),
		Limit:    filter.Limit,
		Offset:   filter.Offset,
	})
	if err != nil {
		return nil, err
	}

	states := make([]glowflow.InstanceState, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		state, err := decodeInstanceState(row)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (s *xdbStorage) DeleteInstanceState(ctx context.Context, instanceID string) error {
	return daos.DeleteInstanceState(ctx, dbResolver(s.dbConnName), instanceID)
}

func (s *xdbStorage) SaveNodeState(ctx context.Context, state glowflow.NodeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return daos.SaveNodeState(ctx, dbResolver(s.dbConnName), &models.NodeState{
		InstanceID: state.InstanceID,
		NodeID:     state.NodeID,
		ChainID:    state.ChainID,
		Version:    state.Version,
		Status:     string(state.Status),
		Attempts:   state.Attempts,
		StateData:  string(data),
	})
}

func (s *xdbStorage) GetNodeState(ctx context.Context, instanceID string, nodeID string) (glowflow.NodeState, error) {
	row, err := daos.GetNodeState(ctx, dbResolver(s.dbConnName), instanceID, nodeID)
	if err != nil {
		if errors.Is(err, gluexdb.ErrEmptyError) {
			return glowflow.NodeState{}, fmt.Errorf("node state %q/%q: %w", instanceID, nodeID, glowflow.ErrStateNotFound)
		}
		return glowflow.NodeState{}, err
	}
	return decodeNodeState(row)
}

func (s *xdbStorage) ListNodeStates(ctx context.Context, filter glowflow.NodeStateFilter) ([]glowflow.NodeState, error) {
	rows, err := daos.ListNodeStates(ctx, dbResolver(s.dbConnName), daos.NodeStateFilter{
		InstanceID: filter.InstanceID,
		ChainID:    filter.ChainID,
		NodeID:     filter.NodeID,
		Statuses:   nodeStatusStrings(filter.Statuses),
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	})
	if err != nil {
		return nil, err
	}

	states := make([]glowflow.NodeState, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		state, err := decodeNodeState(row)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (s *xdbStorage) DeleteNodeState(ctx context.Context, instanceID string, nodeID string) error {
	return daos.DeleteNodeState(ctx, dbResolver(s.dbConnName), instanceID, nodeID)
}

func decodeInstanceState(row *models.InstanceState) (glowflow.InstanceState, error) {
	var state glowflow.InstanceState
	if err := json.Unmarshal([]byte(row.StateData), &state); err != nil {
		return glowflow.InstanceState{}, fmt.Errorf("decode instance state %q: %w", row.InstanceID, err)
	}
	return state, nil
}

func decodeNodeState(row *models.NodeState) (glowflow.NodeState, error) {
	var state glowflow.NodeState
	if err := json.Unmarshal([]byte(row.StateData), &state); err != nil {
		return glowflow.NodeState{}, fmt.Errorf("decode node state %q/%q: %w", row.InstanceID, row.NodeID, err)
	}
	return state, nil
}

func instanceStatusStrings(statuses []glowflow.InstanceStatus) []string {
	if len(statuses) == 0 {
		return nil
	}
	result := make([]string, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, string(status))
	}
	return result
}

func nodeStatusStrings(statuses []glowflow.NodeStatus) []string {
	if len(statuses) == 0 {
		return nil
	}
	result := make([]string, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, string(status))
	}
	return result
}
