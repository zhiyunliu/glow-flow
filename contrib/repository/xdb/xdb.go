package xdb

import (
	"context"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/daos"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/models"
	"github.com/zhiyunliu/glue"
)

// NewRepository creates a new JSON file repository.
func NewRepository(dbConnName string) glowflow.DataRepository {
	return &xdbRepository{
		dbConnName: dbConnName,
	}
}

type xdbRepository struct {
	dbConnName string
}

// LoadChainDefinitions loads the chain definitions from the XDB.
func (r *xdbRepository) LoadChainDefinitions(ctx context.Context) ([]*glowflow.ChainDefinition, error) {
	dbObj := glue.DB(r.dbConnName)
	chainDefs, err := daos.LoadChainDefinitions(ctx, dbObj)
	if err != nil {
		return nil, err
	}

	nodeDefs, err := daos.LoadChainNodes(ctx, dbObj)
	if err != nil {
		return nil, err
	}

	connDefs, err := daos.LoadChainConnections(ctx, dbObj)
	if err != nil {
		return nil, err
	}

	return r.buildChainDefinition(ctx, chainDefs, nodeDefs, connDefs)
}

// LoadChainDefinition loads a specific chain definition from the XDB.
func (r *xdbRepository) LoadChainDefinition(ctx context.Context, chainID string) (*glowflow.ChainDefinition, error) {
	return nil, nil
}

func (r *xdbRepository) buildChainDefinition(ctx context.Context, chainDefs []*models.ChainDefinition, nodeDefs []*models.NodeDefinition, connDefs []*models.ConnectionDefinition) ([]*glowflow.ChainDefinition, error) {
	return nil, nil
}
