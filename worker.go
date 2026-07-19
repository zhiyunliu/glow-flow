package glowflow

type Ability struct {
	Name        string
	Description string
	Concurrency int
}

type Worker interface {
	Name() string
	Ability() map[string]*Ability
	Execute(ctx Context, data any) (ExecuteResult, error)
}
