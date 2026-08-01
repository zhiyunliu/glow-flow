package daos

import (
	"context"

	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/models"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/sqls"
	"github.com/zhiyunliu/glue/xdb"
)

// LoadChainDefinitions loads the chain definitions from the XDB.
func LoadChainDefinitions(ctx context.Context, dbObj xdb.Executer) (result []*models.ChainDefinition, err error) {
	result = make([]*models.ChainDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainDefinitions, nil, &result)
	if err != nil {
		return
	}

	return
}

func LoadChainNodes(ctx context.Context, dbObj xdb.Executer) (result []*models.NodeDefinition, err error) {
	result = make([]*models.NodeDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainNodes, nil, &result)
	return
}
func LoadChainConnections(ctx context.Context, dbObj xdb.Executer) (result []*models.ConnectionDefinition, err error) {
	result = make([]*models.ConnectionDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainConnections, nil, &result)
	return
}
