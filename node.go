package glowflow

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
	Type() string
	IsStartNode() bool
	Config() any
	Position() Position
}

type CompiledNode interface {
	Node
	Execute(ctx Context, data any) (ExecuteResult, error)
}
