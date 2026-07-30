package glowflow

import (
	"context"
	"errors"
	"time"
)

var ErrStateNotFound = errors.New("state not found")

type InstanceStatus string

const (
	InstanceStatusPending   InstanceStatus = "pending"
	InstanceStatusRunning   InstanceStatus = "running"
	InstanceStatusPaused    InstanceStatus = "paused"
	InstanceStatusCompleted InstanceStatus = "completed"
	InstanceStatusFailed    InstanceStatus = "failed"
	InstanceStatusCanceled  InstanceStatus = "canceled"
)

type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusCompleted NodeStatus = "completed"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusSkipped   NodeStatus = "skipped"
)

type InstanceState struct {
	InstanceID     string         `json:"instance_id"`
	FlowID         string         `json:"flow_id"`
	Version        string         `json:"version"`
	Status         InstanceStatus `json:"status"`
	CurrentNodeIDs []string       `json:"current_node_ids,omitempty"`
	Variables      map[string]any `json:"variables,omitempty"`
	Error          string         `json:"error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
}

type NodeState struct {
	InstanceID string         `json:"instance_id"`
	FlowID     string         `json:"flow_id"`
	Version    string         `json:"version"`
	NodeID     string         `json:"node_id"`
	Status     NodeStatus     `json:"status"`
	Attempts   int            `json:"attempts"`
	Input      any            `json:"input,omitempty"`
	Output     any            `json:"output,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

type InstanceStateFilter struct {
	FlowID   string
	Version  string
	Statuses []InstanceStatus
	Limit    int
	Offset   int
}

type NodeStateFilter struct {
	InstanceID string
	FlowID     string
	NodeID     string
	Statuses   []NodeStatus
	Limit      int
	Offset     int
}

type StateStorage interface {
	SaveInstanceState(ctx context.Context, state InstanceState) error
	GetInstanceState(ctx context.Context, instanceID string) (InstanceState, error)
	ListInstanceStates(ctx context.Context, filter InstanceStateFilter) ([]InstanceState, error)
	DeleteInstanceState(ctx context.Context, instanceID string) error

	SaveNodeState(ctx context.Context, state NodeState) error
	GetNodeState(ctx context.Context, instanceID string, nodeID string) (NodeState, error)
	ListNodeStates(ctx context.Context, filter NodeStateFilter) ([]NodeState, error)
	DeleteNodeState(ctx context.Context, instanceID string, nodeID string) error
}
