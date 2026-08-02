package daos

import (
	"context"

	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/models"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/sqls"
	"github.com/zhiyunliu/glue/xdb"
)

type chainNoParam struct {
	ChainNo string `json:"chain_no"`
}

type infraNoParam struct {
	InfraNo string `json:"infra_no"`
}

// LoadChainDefinitions loads the chain definitions from the XDB.
func LoadChainDefinitions(ctx context.Context, dbObj xdb.Executer) (result []*models.ChainDefinition, err error) {
	result = make([]*models.ChainDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainDefinitions, nil, &result)
	if err != nil {
		return
	}

	return
}

func LoadChainDefinition(ctx context.Context, dbObj xdb.Executer, chainNo string) (result []*models.ChainDefinition, err error) {
	result = make([]*models.ChainDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainDefinition, chainNoParam{ChainNo: chainNo}, &result)
	return
}

func LoadChainNodes(ctx context.Context, dbObj xdb.Executer) (result []*models.NodeDefinition, err error) {
	result = make([]*models.NodeDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainNodes, nil, &result)
	return
}

func LoadChainNodesByChainNo(ctx context.Context, dbObj xdb.Executer, chainNo string) (result []*models.NodeDefinition, err error) {
	result = make([]*models.NodeDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainNodesByChainNo, chainNoParam{ChainNo: chainNo}, &result)
	return
}

func LoadChainConnections(ctx context.Context, dbObj xdb.Executer) (result []*models.ConnectionDefinition, err error) {
	result = make([]*models.ConnectionDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainConnections, nil, &result)
	return
}

func LoadChainConnectionsByChainNo(ctx context.Context, dbObj xdb.Executer, chainNo string) (result []*models.ConnectionDefinition, err error) {
	result = make([]*models.ConnectionDefinition, 0)
	err = dbObj.QueryAs(ctx, sqls.LoadChainConnectionsByChainNo, chainNoParam{ChainNo: chainNo}, &result)
	return
}

func LoadBasicInfra(ctx context.Context, dbObj xdb.Executer, infraNo string) (result *models.BasicInfra, err error) {
	result = &models.BasicInfra{}
	err = dbObj.FirstAs(ctx, sqls.LoadBasicInfra, infraNoParam{InfraNo: infraNo}, result)
	return
}
