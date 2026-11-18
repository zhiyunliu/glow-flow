package models

// InstanceState 对应 dbo.flow_instance_state 表的一行记录。
type InstanceState struct {
	InstanceID string `json:"instance_id"`
	ChainID    string `json:"chain_id"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	StateData  string `json:"state_data"`
}

// NodeState 对应 dbo.flow_node_state 表的一行记录。
type NodeState struct {
	InstanceID string `json:"instance_id"`
	NodeID     string `json:"node_id"`
	ChainID    string `json:"chain_id"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Attempts   int    `json:"attempts"`
	StateData  string `json:"state_data"`
}
