package models

// RunFlowParams 执行流程
type RunFlowParams struct {
	Chain string `json:"chain" binding:"required"` // 流程ID
	Data  any    `json:"data"`                     // 消息数据
}

// ChainInfo 已加载的流程
type ChainInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Nodes   int    `json:"nodes"`
}

// ReloadResult 重新加载流程定义的结果；编译失败的流程不影响其他流程
type ReloadResult struct {
	Loaded []string          `json:"loaded"`
	Failed map[string]string `json:"failed,omitempty"` // 流程ID -> 错误信息
}

// RunResult 一次流程执行的结果
type RunResult struct {
	ChainID    string       `json:"chain_id"`
	InstanceID string       `json:"instance_id,omitempty"`
	Error      string       `json:"error,omitempty"` // 节点返回 error 或 panic 时 dispatcher 中断执行的错误
	CostMs     int64        `json:"cost_ms"`
	Results    []any        `json:"results"` // 流程中 example.collect 节点收集的结果
	Trace      []TraceEvent `json:"trace"`   // 节点执行轨迹，按开始顺序排列
}

// TraceEvent 单个节点的一次执行记录
type TraceEvent struct {
	Seq       int64    `json:"seq"`
	NodeID    string   `json:"node_id"`
	NodeName  string   `json:"node_name"`
	NodeType  string   `json:"node_type"`
	LoopIndex *int     `json:"loop_index,omitempty"` // 在 for/while 循环体中执行时的迭代序号
	Relation  string   `json:"relation,omitempty"`
	Relations []string `json:"relations,omitempty"` // inclusive 节点同时走的多个分支
	End       bool     `json:"end,omitempty"`
	Output    any      `json:"output,omitempty"`
	Error     string   `json:"error,omitempty"`
	CostMs    int64    `json:"cost_ms"`
}
