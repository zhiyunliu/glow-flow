package action

import (
	"context"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/constants/enums/relationtype"
	"github.com/zhiyunliu/glow-flow/errors"
	"github.com/zhiyunliu/glue"
	"github.com/zhiyunliu/glue/xdb"
	"github.com/zhiyunliu/golibs/xtypes"
)

//type xdbStreamReader = xdb.RowDataReader

type xdbNode struct {
	glowflow.Node
	dbconn     string
	execSql    string
	execMethod string
	stream     bool
	xdbMethods map[string]xdbExecCallback
}
type xdbExecCallback func(ctx context.Context, node *xdbNode, dbObj xdb.Executer, sql string, data any) (result any, err error)

var (
	xdbMethods = map[string]xdbExecCallback{
		"queryas": func(ctx context.Context, node *xdbNode, dbObj xdb.Executer, sql string, data any) (result any, err error) {
			result = &[]xtypes.XMap{}
			if node.stream {
				dataChan := make(chan *xtypes.XMap)
				result = xdb.NewStreamChan(dataChan)
			}
			err = dbObj.QueryAs(ctx, sql, data, result)
			return
		},
		"firstas": func(ctx context.Context, node *xdbNode, dbObj xdb.Executer, sql string, data any) (result any, err error) {

			result = &xtypes.XMap{}
			if node.stream {
				dataChan := make(chan *xtypes.XMap)
				result = xdb.NewStreamChan(dataChan)
			}
			err = dbObj.FirstAs(ctx, sql, data, result)
			return
		},
		"exec": func(ctx context.Context, node *xdbNode, dbObj xdb.Executer, sql string, data any) (result any, err error) {
			result, err = dbObj.Exec(ctx, sql, data)
			return
		},
	}
)

func (n *xdbNode) Execute(ctx glowflow.Context, data any) (glowflow.ExecuteResult, error) {

	xdbExec, ok := n.xdbMethods[n.execMethod]
	if !ok {
		return glowflow.ExecuteResult{
			RelationType: relationtype.Failure,
			Data:         nil,
		}, nil
	}

	dbObj := glue.DB(n.dbconn)
	result, err := xdbExec(ctx, n, dbObj, n.execSql, data)
	if err != nil {
		return glowflow.ExecuteResult{
			RelationType: relationtype.Failure,
			Streamable:   n.stream,
			Data:         nil,
		}, nil
	}

	return glowflow.ExecuteResult{
		RelationType: relationtype.Success,
		Streamable:   n.stream,
		Data:         result,
	}, nil
}

type xdbFactory struct{}

func (f *xdbFactory) Build(def glowflow.NodeDefinition) (glowflow.CompiledNode, error) {
	if len(def.ExtParams) == 0 {
		return nil, errors.ErrInvalidNodeConfig
	}

	return &xdbNode{
		Node:       def,
		dbconn:     def.ExtParams.GetString("dbconn"),
		execSql:    def.ExtParams.GetString("execsql"),
		execMethod: def.ExtParams.GetString("execmethod"),
		stream:     def.ExtParams.GetBool("stream"),
		xdbMethods: xdbMethods,
	}, nil
}

func init() {
	glowflow.DefaultRegistry.Register(&glowflow.NodeDescriptor{
		Type:    "xdb",
		Factory: &xdbFactory{},
	})
}
