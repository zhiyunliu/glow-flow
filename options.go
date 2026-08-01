package glowflow

type options struct {
	dataRepository DataRepository
	stateStorage   StateStorage
	registry       Registry
	compiler       ChainCompiler
	dispatcher     Dispatcher
}

// Option is a function that configures the engine.
type Option func(*options)

// WithDataRepository sets the data repository for the engine.
func WithDataRepository(repo DataRepository) Option {
	return func(o *options) {
		o.dataRepository = repo
	}
}

// WithStateStorage sets the state storage for the engine.
func WithStateStorage(storage StateStorage) Option {
	return func(o *options) {
		o.stateStorage = storage
	}
}

func WithRegistry(registry Registry) Option {
	return func(o *options) {
		o.registry = registry
	}
}

func WithChainCompiler(compiler ChainCompiler) Option {
	return func(o *options) {
		o.compiler = compiler
	}
}

func WithDispatcher(dispatcher Dispatcher) Option {
	return func(o *options) {
		o.dispatcher = dispatcher
	}
}
