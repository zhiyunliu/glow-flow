package glowflow

import "context"

// DataRepository is the interface for the data repository.
var DefaultRepository DataRepository

type DataRepository interface {
	LoadChainDefinitions(ctx context.Context) ([]*ChainDefinition, error)
	LoadChainDefinition(ctx context.Context, chainNo string) (*ChainDefinition, error)
	LoadBasicInfra(ctx context.Context, infraNo string) (*BasicInfra, error)
}
