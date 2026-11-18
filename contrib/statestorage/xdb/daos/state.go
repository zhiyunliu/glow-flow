package daos

import (
	"context"

	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/models"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/sqls"
	"github.com/zhiyunliu/glue/xdb"
)

type instanceIDParam struct {
	InstanceID string `json:"instance_id"`
}

type nodeKeyParam struct {
	InstanceID string `json:"instance_id"`
	NodeID     string `json:"node_id"`
}

// InstanceStateFilter 实例状态查询条件，空值条件不参与过滤。
type InstanceStateFilter struct {
	ChainID  string
	Version  string
	Statuses []string
	Limit    int
	Offset   int
}

// NodeStateFilter 节点状态查询条件，空值条件不参与过滤；NodeID 为模糊匹配。
type NodeStateFilter struct {
	InstanceID string
	ChainID    string
	NodeID     string
	Statuses   []string
	Limit      int
	Offset     int
}

func SaveInstanceState(ctx context.Context, dbObj xdb.Executer, state *models.InstanceState) (err error) {
	_, err = dbObj.Exec(ctx, sqls.SaveInstanceState, state)
	return
}

// GetInstanceState 查询实例状态，记录不存在时返回 xdb.ErrEmptyError。
func GetInstanceState(ctx context.Context, dbObj xdb.Executer, instanceID string) (result *models.InstanceState, err error) {
	result = &models.InstanceState{}
	err = dbObj.FirstAs(ctx, sqls.GetInstanceState, instanceIDParam{InstanceID: instanceID}, result)
	return
}

func ListInstanceStates(ctx context.Context, dbObj xdb.Executer, filter InstanceStateFilter) (result []*models.InstanceState, err error) {
	result = make([]*models.InstanceState, 0)
	query := sqls.ListInstanceStates
	if filter.Limit > 0 {
		query = sqls.ListInstanceStatesWithLimit
	}
	params := map[string]any{
		"chain_id": filter.ChainID,
		"version":  filter.Version,
		"status":   filter.Statuses,
		"offset":   normalizeOffset(filter.Offset),
		"limit":    filter.Limit,
	}
	err = dbObj.QueryAs(ctx, query, params, &result)
	return
}

func DeleteInstanceState(ctx context.Context, dbObj xdb.Executer, instanceID string) (err error) {
	_, err = dbObj.Exec(ctx, sqls.DeleteInstanceState, instanceIDParam{InstanceID: instanceID})
	return
}

func SaveNodeState(ctx context.Context, dbObj xdb.Executer, state *models.NodeState) (err error) {
	_, err = dbObj.Exec(ctx, sqls.SaveNodeState, state)
	return
}

// GetNodeState 查询节点状态，记录不存在时返回 xdb.ErrEmptyError。
func GetNodeState(ctx context.Context, dbObj xdb.Executer, instanceID string, nodeID string) (result *models.NodeState, err error) {
	result = &models.NodeState{}
	err = dbObj.FirstAs(ctx, sqls.GetNodeState, nodeKeyParam{InstanceID: instanceID, NodeID: nodeID}, result)
	return
}

func ListNodeStates(ctx context.Context, dbObj xdb.Executer, filter NodeStateFilter) (result []*models.NodeState, err error) {
	result = make([]*models.NodeState, 0)
	query := sqls.ListNodeStates
	if filter.Limit > 0 {
		query = sqls.ListNodeStatesWithLimit
	}
	params := map[string]any{
		"instance_id": filter.InstanceID,
		"chain_id":    filter.ChainID,
		"node_id":     filter.NodeID,
		"status":      filter.Statuses,
		"offset":      normalizeOffset(filter.Offset),
		"limit":       filter.Limit,
	}
	err = dbObj.QueryAs(ctx, query, params, &result)
	return
}

func DeleteNodeState(ctx context.Context, dbObj xdb.Executer, instanceID string, nodeID string) (err error) {
	_, err = dbObj.Exec(ctx, sqls.DeleteNodeState, nodeKeyParam{InstanceID: instanceID, NodeID: nodeID})
	return
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
