package glowflow

type ExecuteResult struct {
	RelationType string
	Streamable   bool
	Msg          *Message
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
	Execute(ctx Context, msg *Message) (ExecuteResult, error)
}
