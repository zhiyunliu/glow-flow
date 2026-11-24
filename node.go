package glowflow

import "time"

type ExecuteResult struct {
	RelationType string
	// RelationTypes 同时走多个关系分支(如 inclusive 节点)；非空时替代 RelationType
	RelationTypes []string
	// End 结束当前分支：不再执行后续节点，以当前结果作为分支结果(如 end 节点)
	End        bool
	Streamable bool
	Msg        *Message
	Error      error
}

type Node interface {
	Id() string
	Name() string
	Type() string
	Config() any
	Layout() Layout
}

type CompiledNode interface {
	Node
	Execute(ctx Context, msg *Message) (ExecuteResult, error)
}

type NodeCall func(ctx Context, msg *Message) (ExecuteResult, error)

// JoinInput 汇聚节点收到的单个分支消息
type JoinInput struct {
	FromID string // 消息来源节点ID；起始节点或通过 ExecuteNode 直接执行时为空
	Msg    *Message
}

// JoinNode 汇聚节点：dispatcher 收集所有可能到达该节点的分支消息后只调用一次 Join。
//
// 当前执行作用域中不再有可到达该节点的执行中节点时即视为收集完成，因此条件分支未走到的路径不会阻塞汇聚；
// JoinTimeout()>0 时，超时后以已收到的消息调用 Join(timedOut=true)，之后到达的消息被丢弃。
type JoinNode interface {
	CompiledNode
	JoinTimeout() time.Duration
	Join(ctx Context, inputs []JoinInput, timedOut bool) (ExecuteResult, error)
}

// NodeReferrer 通过节点ID引用流程中其他节点的节点(如 for/while 的 do、groupaction 的 nodeids)。
// 编译时校验被引用节点存在；被引用节点不作为流程起始节点，由引用方通过 ExecuteNode 按需执行。
type NodeReferrer interface {
	ReferencedNodes() []string
}
