package xdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/daos"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/models"
	"github.com/zhiyunliu/glue"
	gluexdb "github.com/zhiyunliu/glue/xdb"
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

var dbResolver = func(name string) gluexdb.Executer {
	return glue.DB(name)
}

// LoadChainDefinitions loads the chain definitions from the XDB.
func (r *xdbRepository) LoadChainDefinitions(ctx context.Context) ([]*glowflow.ChainDefinition, error) {
	dbObj := dbResolver(r.dbConnName)
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
func (r *xdbRepository) LoadChainDefinition(ctx context.Context, chainNo string) (*glowflow.ChainDefinition, error) {
	dbObj := dbResolver(r.dbConnName)
	chainDefs, err := daos.LoadChainDefinition(ctx, dbObj, chainNo)
	if err != nil {
		return nil, err
	}

	nodeDefs, err := daos.LoadChainNodesByChainNo(ctx, dbObj, chainNo)
	if err != nil {
		return nil, err
	}

	connDefs, err := daos.LoadChainConnectionsByChainNo(ctx, dbObj, chainNo)
	if err != nil {
		return nil, err
	}

	definitions, err := r.buildChainDefinition(ctx, chainDefs, nodeDefs, connDefs)
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("chain definition %q not found", chainNo)
	}
	return definitions[0], nil
}

// LoadBasicInfra loads a specific basic infrastructure definition from the XDB.
func (r *xdbRepository) LoadBasicInfra(ctx context.Context, infraNo string) (*glowflow.BasicInfra, error) {
	dbObj := dbResolver(r.dbConnName)
	infra, err := daos.LoadBasicInfra(ctx, dbObj, infraNo)
	if err != nil {
		return nil, err
	}
	if infra == nil || infra.InfraNo == "" {
		return nil, fmt.Errorf("basic infra %q not found", infraNo)
	}
	return r.newBasicInfra(infra)
}

func (r *xdbRepository) Close() error {
	// No resources to close for XDB repository
	return nil
}

func (r *xdbRepository) buildChainDefinition(ctx context.Context, chainDefs []*models.ChainDefinition, nodeDefs []*models.NodeDefinition, connDefs []*models.ConnectionDefinition) ([]*glowflow.ChainDefinition, error) {
	_ = ctx
	selected := make(map[string]*models.ChainDefinition)
	for _, chainDef := range chainDefs {
		if chainDef == nil || chainDef.Status != 1 {
			continue
		}
		current := selected[chainDef.ChainNo]
		if current == nil || chainDef.VersionID > current.VersionID {
			selected[chainDef.ChainNo] = chainDef
		}
	}

	chainNos := make([]string, 0, len(selected))
	for chainNo := range selected {
		chainNos = append(chainNos, chainNo)
	}
	sort.Strings(chainNos)

	definitions := make([]*glowflow.ChainDefinition, 0, len(chainNos))
	for _, chainNo := range chainNos {
		chainDef := selected[chainNo]
		definition, err := r.newChainDefinition(chainDef)
		if err != nil {
			return nil, err
		}

		nodes, err := r.buildNodes(chainDef, nodeDefs)
		if err != nil {
			return nil, err
		}
		definition.Nodes = nodes

		definition.Connections = r.buildConnections(chainDef, connDefs)
		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func (r *xdbRepository) newChainDefinition(chainDef *models.ChainDefinition) (*glowflow.ChainDefinition, error) {
	extParams, err := decodeExtParams(chainDef.ExtParams, fmt.Sprintf("chain %s extparams", chainDef.ChainNo))
	if err != nil {
		return nil, err
	}
	layout, err := decodeLayout(chainDef.Layout, fmt.Sprintf("chain %s layout", chainDef.ChainNo))
	if err != nil {
		return nil, err
	}
	return &glowflow.ChainDefinition{
		ID:      chainDef.ChainNo,
		Version: strconv.FormatInt(chainDef.VersionID, 10),
		Metadata: glowflow.ChainMetadata{
			ID:        chainDef.ChainNo,
			Name:      chainDef.Name,
			Status:    chainDef.Status,
			ExtParams: extParams,
			Layout:    layout,
		},
		Endpoints:   []glowflow.EndpointDefinition{},
		Nodes:       []glowflow.NodeDefinition{},
		Connections: []glowflow.ConnectionDefinition{},
	}, nil
}

func (r *xdbRepository) buildNodes(chainDef *models.ChainDefinition, nodeDefs []*models.NodeDefinition) ([]glowflow.NodeDefinition, error) {
	nodes := make([]*models.NodeDefinition, 0)
	for _, nodeDef := range nodeDefs {
		if nodeDef == nil || nodeDef.ChainNo != chainDef.ChainNo || nodeDef.VersionID != chainDef.VersionID {
			continue
		}
		nodes = append(nodes, nodeDef)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].NodeDefNo != nodes[j].NodeDefNo {
			return nodes[i].NodeDefNo < nodes[j].NodeDefNo
		}
		return nodes[i].NodeID < nodes[j].NodeID
	})

	definitions := make([]glowflow.NodeDefinition, 0, len(nodes))
	for _, nodeDef := range nodes {
		extParams, err := decodeNodeExtParams(nodeDef)
		if err != nil {
			return nil, err
		}
		layout, err := decodeNodeLayout(nodeDef)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, glowflow.NodeDefinition{
			ID:        nodeDef.NodeDefNo,
			Type:      nodeDef.Type,
			Name:      nodeDef.Name,
			Layout:    layout,
			ExtParams: extParams,
		})
	}
	return definitions, nil
}

func (r *xdbRepository) buildConnections(chainDef *models.ChainDefinition, connDefs []*models.ConnectionDefinition) []glowflow.ConnectionDefinition {
	connections := make([]*models.ConnectionDefinition, 0)
	for _, connDef := range connDefs {
		if connDef == nil || connDef.ChainNo != chainDef.ChainNo || connDef.VersionID != chainDef.VersionID {
			continue
		}
		connections = append(connections, connDef)
	}
	sort.SliceStable(connections, func(i, j int) bool {
		return connections[i].ConnID < connections[j].ConnID
	})

	definitions := make([]glowflow.ConnectionDefinition, 0, len(connections))
	for _, connDef := range connections {
		definitions = append(definitions, glowflow.ConnectionDefinition{
			FromID: connDef.FromID,
			ToID:   connDef.ToID,
			Type:   connDef.Type,
			Remark: nullStringValue(connDef.Remark),
		})
	}
	return definitions
}

func (r *xdbRepository) newBasicInfra(infra *models.BasicInfra) (*glowflow.BasicInfra, error) {
	extParams, err := decodeExtParams(infra.ExtParams, fmt.Sprintf("basic infra %s extparams", infra.InfraNo))
	if err != nil {
		return nil, err
	}
	return &glowflow.BasicInfra{
		InfraNo:   infra.InfraNo,
		Name:      infra.InfraNo,
		Type:      infra.InfraType,
		Desc:      nullStringValue(infra.Desc),
		ExtParams: extParams,
	}, nil
}

func decodeNodeExtParams(nodeDef *models.NodeDefinition) (map[string]any, error) {
	if hasString(nodeDef.ExtParams) {
		return decodeExtParams(nodeDef.ExtParams, fmt.Sprintf("node %s extparams", nodeDef.NodeDefNo))
	}
	return decodeExtParams(nodeDef.DefinitionParams, fmt.Sprintf("node %s definition extparams", nodeDef.NodeDefNo))
}

func decodeNodeLayout(nodeDef *models.NodeDefinition) (glowflow.Layout, error) {
	if hasString(nodeDef.Layout) {
		return decodeLayout(nodeDef.Layout, fmt.Sprintf("node %s layout", nodeDef.NodeDefNo))
	}
	return decodeLayout(nodeDef.LayoutDefinition, fmt.Sprintf("node %s definition layout", nodeDef.NodeDefNo))
}

func decodeExtParams(value sql.NullString, label string) (map[string]any, error) {
	if !hasString(value) {
		return nil, nil
	}
	if isJSONNull(value.String) {
		return nil, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(value.String), &result); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return result, nil
}

func decodeLayout(value sql.NullString, label string) (glowflow.Layout, error) {
	if !hasString(value) {
		return glowflow.Layout{}, nil
	}
	if isJSONNull(value.String) {
		return glowflow.Layout{}, nil
	}
	var result glowflow.Layout
	if err := json.Unmarshal([]byte(value.String), &result); err != nil {
		return glowflow.Layout{}, fmt.Errorf("decode %s: %w", label, err)
	}
	return result, nil
}

func hasString(value sql.NullString) bool {
	return value.Valid && strings.TrimSpace(value.String) != ""
}

func isJSONNull(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "null")
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
