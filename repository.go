package glowflow

// DataRepository is the interface for the data repository.
var DefaultDataRepository DataRepository

type DataRepository interface {
	LoadChainDefinitions() ([]*ChainDefinition, error)
	LoadChainDefinition(chainID string) (*ChainDefinition, error)
}
