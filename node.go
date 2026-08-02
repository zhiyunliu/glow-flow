package glowflow

type ExecuteResult struct {
	RelationType string
	Data         any
	Error        error
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
	Execute(ctx Context, data any) (ExecuteResult, error)
}
