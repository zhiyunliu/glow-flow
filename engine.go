package glowflow

type Engine struct {
	options *options
}

// NewEngine creates a new engine with the given options.
func NewEngine(opts ...Option) *Engine {
	engine := &Engine{options: &options{}}
	for _, opt := range opts {
		opt(engine.options)
	}
	return engine
}

// Run runs the engine.
func (e *Engine) Run() error {
	return nil
}

// Stop stops the engine.
func (e *Engine) Stop() error {
	return nil
}
