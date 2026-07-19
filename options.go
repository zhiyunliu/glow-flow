package glowflow

type options struct {
	dataRepository DataRepository
	stateStorage   StateStorage
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
