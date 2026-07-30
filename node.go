package glowflow

import (
	"github.com/zhiyunliu/glow-flow/nodetype"
)

// Position is the position of the node in the flow.
type Position struct {
	X, Y int
}

type ExecuteResult struct {
	RelationType string
	Data         any
	Error        error
}

type Node interface {
	Id() string
	Name() string
	Type() nodetype.NodeType
	IsStartNode() bool
	Config() any
	Position() Position
}

type CompiledNode interface {
	Node
	Execute(ctx Context, data any) (ExecuteResult, error)
}
