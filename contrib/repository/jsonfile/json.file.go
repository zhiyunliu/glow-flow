package jsonfile

import (
	"context"

	glowflow "github.com/zhiyunliu/glow-flow"
)

// NewRepository creates a new JSON file repository.
func NewRepository() glowflow.DataRepository {
	return &jsonFileRepository{}
}

type jsonFileRepository struct {
}

func (r *jsonFileRepository) LoadChainDefinitions(ctx context.Context) ([]*glowflow.ChainDefinition, error) {
	return nil, nil
}

func (r *jsonFileRepository) LoadChainDefinition(ctx context.Context, chainID string) (*glowflow.ChainDefinition, error) {
	return nil, nil
}
