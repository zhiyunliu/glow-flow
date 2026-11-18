package xdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/models"
	"github.com/zhiyunliu/glow-flow/contrib/statestorage/xdb/sqls"
	gluexdb "github.com/zhiyunliu/glue/xdb"
)

func TestXDBStorageInstanceStateContract(t *testing.T) {
	ctx := context.Background()
	storage, dbObj := newTestXDBStorage(t)
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
			Variables:      map[string]any{"count": float64(1), "mode": "fast", "中文": "值"},
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

	// 再次保存同一实例应覆盖原记录。
	updated := states[1]
	updated.Status = glowflow.InstanceStatusCompleted
	updated.FinishedAt = &finishedAt
	if err := storage.SaveInstanceState(ctx, updated); err != nil {
		t.Fatalf("update instance state: %v", err)
	}
	if got, _ := storage.GetInstanceState(ctx, "inst-001"); !reflect.DeepEqual(got, updated) {
		t.Fatalf("updated instance state = %#v, want %#v", got, updated)
	}
	if err := storage.SaveInstanceState(ctx, states[1]); err != nil {
		t.Fatalf("restore instance state: %v", err)
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
	if dbObj.lastQuery != sqls.ListInstanceStatesWithLimit {
		t.Fatalf("limited list should use ListInstanceStatesWithLimit")
	}

	all, err := storage.ListInstanceStates(ctx, glowflow.InstanceStateFilter{Offset: -1})
	if err != nil {
		t.Fatalf("list all instance states: %v", err)
	}
	if gotIDs := instanceStateIDs(all); !reflect.DeepEqual(gotIDs, []string{"inst-001", "inst-002", "inst-003", "inst-004"}) {
		t.Fatalf("all instance IDs = %#v", gotIDs)
	}
	if dbObj.lastQuery != sqls.ListInstanceStates {
		t.Fatalf("unlimited list should use ListInstanceStates")
	}
	if dbObj.lastParams["offset"] != 0 {
		t.Fatalf("negative offset should be normalized to 0, got %#v", dbObj.lastParams["offset"])
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

func TestXDBStorageNodeStateContract(t *testing.T) {
	ctx := context.Background()
	storage, _ := newTestXDBStorage(t)
	startedAt := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)

	states := []glowflow.NodeState{
		{
			InstanceID: "inst-001",
			ChainID:    "flow-a",
			Version:    "v1",
			NodeID:     "node-c",
			Status:     glowflow.NodeStatusSkipped,
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

func TestXDBStorageReturnsDecodeErrorForMalformedStateData(t *testing.T) {
	ctx := context.Background()
	storage, dbObj := newTestXDBStorage(t)
	dbObj.instances["inst-bad"] = &models.InstanceState{InstanceID: "inst-bad", StateData: "{bad"}
	dbObj.nodes["inst-bad/node-bad"] = &models.NodeState{InstanceID: "inst-bad", NodeID: "node-bad", StateData: "{bad"}

	if _, err := storage.GetInstanceState(ctx, "inst-bad"); err == nil || errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("malformed instance state error = %v, want decode error", err)
	}
	if _, err := storage.ListInstanceStates(ctx, glowflow.InstanceStateFilter{}); err == nil {
		t.Fatalf("malformed instance list should return decode error")
	}
	if _, err := storage.GetNodeState(ctx, "inst-bad", "node-bad"); err == nil || errors.Is(err, glowflow.ErrStateNotFound) {
		t.Fatalf("malformed node state error = %v, want decode error", err)
	}
}

func TestXDBStoragePropagatesDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	storage, dbObj := newTestXDBStorage(t)
	dbObj.err = errors.New("db down")

	if _, err := storage.GetInstanceState(ctx, "inst-001"); !errors.Is(err, dbObj.err) {
		t.Fatalf("get instance error = %v, want db error", err)
	}
	if err := storage.SaveNodeState(ctx, glowflow.NodeState{InstanceID: "i", NodeID: "n"}); !errors.Is(err, dbObj.err) {
		t.Fatalf("save node error = %v, want db error", err)
	}
}

func newTestXDBStorage(t *testing.T) (glowflow.StateStorage, *memoryExecuter) {
	t.Helper()
	dbObj := &memoryExecuter{
		instances: map[string]*models.InstanceState{},
		nodes:     map[string]*models.NodeState{},
	}
	original := dbResolver
	dbResolver = func(name string) gluexdb.Executer {
		if name != "flow" {
			t.Fatalf("db conn name = %q, want flow", name)
		}
		return dbObj
	}
	t.Cleanup(func() { dbResolver = original })
	return NewStateStorage("flow"), dbObj
}

// memoryExecuter 按 SQL 常量模拟 dbo.flow_instance_state / dbo.flow_node_state 两张表。
type memoryExecuter struct {
	instances  map[string]*models.InstanceState
	nodes      map[string]*models.NodeState
	lastQuery  string
	lastParams map[string]any
	err        error
}

func (e *memoryExecuter) Query(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Rows, error) {
	return nil, fmt.Errorf("unexpected Query")
}

func (e *memoryExecuter) Multi(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) ([]gluexdb.Rows, error) {
	return nil, fmt.Errorf("unexpected Multi")
}

func (e *memoryExecuter) First(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Row, error) {
	return nil, fmt.Errorf("unexpected First")
}

func (e *memoryExecuter) Scalar(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (interface{}, error) {
	return nil, fmt.Errorf("unexpected Scalar")
}

func (e *memoryExecuter) Exec(ctx context.Context, query string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Result, error) {
	if e.err != nil {
		return nil, e.err
	}
	params := toParams(input)
	e.lastQuery, e.lastParams = query, params
	instanceID, _ := params["instance_id"].(string)
	nodeID, _ := params["node_id"].(string)
	switch query {
	case sqls.SaveInstanceState:
		row := &models.InstanceState{}
		remarshal(input, row)
		e.instances[instanceID] = row
	case sqls.DeleteInstanceState:
		delete(e.instances, instanceID)
	case sqls.SaveNodeState:
		row := &models.NodeState{}
		remarshal(input, row)
		e.nodes[instanceID+"/"+nodeID] = row
	case sqls.DeleteNodeState:
		delete(e.nodes, instanceID+"/"+nodeID)
	default:
		return nil, fmt.Errorf("unexpected Exec sql:\n%s", query)
	}
	return nil, nil
}

func (e *memoryExecuter) FirstAs(ctx context.Context, query string, input any, result any, opts ...gluexdb.TemplateOption) error {
	if e.err != nil {
		return e.err
	}
	params := toParams(input)
	e.lastQuery, e.lastParams = query, params
	instanceID, _ := params["instance_id"].(string)
	nodeID, _ := params["node_id"].(string)
	switch query {
	case sqls.GetInstanceState:
		row, ok := e.instances[instanceID]
		if !ok {
			return gluexdb.ErrEmptyError
		}
		*result.(*models.InstanceState) = *row
	case sqls.GetNodeState:
		row, ok := e.nodes[instanceID+"/"+nodeID]
		if !ok {
			return gluexdb.ErrEmptyError
		}
		*result.(*models.NodeState) = *row
	default:
		return fmt.Errorf("unexpected FirstAs sql:\n%s", query)
	}
	return nil
}

func (e *memoryExecuter) QueryAs(ctx context.Context, query string, input any, result any, opts ...gluexdb.TemplateOption) error {
	if e.err != nil {
		return e.err
	}
	params := input.(map[string]any)
	e.lastQuery, e.lastParams = query, params
	offset := params["offset"].(int)
	limit := 0
	if query == sqls.ListInstanceStatesWithLimit || query == sqls.ListNodeStatesWithLimit {
		limit = params["limit"].(int)
	}
	statuses, _ := params["status"].([]string)

	switch query {
	case sqls.ListInstanceStates, sqls.ListInstanceStatesWithLimit:
		rows := make([]*models.InstanceState, 0)
		for _, row := range e.instances {
			if !matchEq(params, "chain_id", row.ChainID) || !matchEq(params, "version", row.Version) || !matchIn(statuses, row.Status) {
				continue
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].InstanceID < rows[j].InstanceID })
		*result.(*[]*models.InstanceState) = paginate(rows, offset, limit)
	case sqls.ListNodeStates, sqls.ListNodeStatesWithLimit:
		rows := make([]*models.NodeState, 0)
		for _, row := range e.nodes {
			nodeID, _ := params["node_id"].(string)
			if !matchEq(params, "instance_id", row.InstanceID) || !matchEq(params, "chain_id", row.ChainID) ||
				!strings.Contains(row.NodeID, nodeID) || !matchIn(statuses, row.Status) {
				continue
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].InstanceID != rows[j].InstanceID {
				return rows[i].InstanceID < rows[j].InstanceID
			}
			return rows[i].NodeID < rows[j].NodeID
		})
		*result.(*[]*models.NodeState) = paginate(rows, offset, limit)
	default:
		return fmt.Errorf("unexpected QueryAs sql:\n%s", query)
	}
	return nil
}

func toParams(input any) map[string]any {
	if m, ok := input.(map[string]any); ok {
		return m
	}
	params := map[string]any{}
	remarshal(input, &params)
	return params
}

func remarshal(src any, dst any) {
	data, err := json.Marshal(src)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		panic(err)
	}
}

func matchEq(params map[string]any, name string, value string) bool {
	want, _ := params[name].(string)
	return want == "" || want == value
}

func matchIn(statuses []string, status string) bool {
	if len(statuses) == 0 {
		return true
	}
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}

func paginate[T any](rows []T, offset int, limit int) []T {
	if offset >= len(rows) {
		return []T{}
	}
	rows = rows[offset:]
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	return rows
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
