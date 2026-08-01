package daos

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/sqls"
	"github.com/zhiyunliu/glue/xdb"
)

func TestXDBDAOQueriesUseChainNoAndSchemaTables(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		required []string
	}{
		{
			name:  "LoadChainDefinitions",
			query: sqls.LoadChainDefinitions,
			required: []string{
				"--sql",
				"flow_chain_definition",
				"flow_chain_version",
				"chain_no",
				"version_id",
				"status",
			},
		},
		{
			name:  "LoadChainNodes",
			query: sqls.LoadChainNodes,
			required: []string{
				"--sql",
				"flow_chain_node",
				"flow_node_definition",
				"chain_no",
				"version_id",
				"node_def_no",
			},
		},
		{
			name:  "LoadChainConnections",
			query: sqls.LoadChainConnections,
			required: []string{
				"--sql",
				"flow_chain_connection",
				"chain_no",
				"version_id",
				"from_id",
				"to_id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := strings.ToLower(strings.TrimSpace(tt.query))
			if query == "" {
				t.Fatalf("%s SQL is empty; expected schema-backed query using chain_no", tt.name)
			}
			if strings.Contains(query, "chain_id") {
				t.Fatalf("%s SQL must use chain_no and must not reference chain_id: %s", tt.name, tt.query)
			}
			for _, required := range tt.required {
				if !strings.Contains(query, required) {
					t.Fatalf("%s SQL missing %q: %s", tt.name, required, tt.query)
				}
			}
		})
	}
}

func TestXDBDAOScopedLoadersPassChainNoParameter(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		call func(context.Context, xdb.Executer) error
		sql  string
	}{
		{
			name: "LoadChainDefinition",
			call: func(ctx context.Context, dbObj xdb.Executer) error {
				_, err := LoadChainDefinition(ctx, dbObj, "chain-a")
				return err
			},
			sql: sqls.LoadChainDefinition,
		},
		{
			name: "LoadChainNodesByChainNo",
			call: func(ctx context.Context, dbObj xdb.Executer) error {
				_, err := LoadChainNodesByChainNo(ctx, dbObj, "chain-a")
				return err
			},
			sql: sqls.LoadChainNodesByChainNo,
		},
		{
			name: "LoadChainConnectionsByChainNo",
			call: func(ctx context.Context, dbObj xdb.Executer) error {
				_, err := LoadChainConnectionsByChainNo(ctx, dbObj, "chain-a")
				return err
			},
			sql: sqls.LoadChainConnectionsByChainNo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbObj := &recordingExecuter{}
			if err := tt.call(ctx, dbObj); err != nil {
				t.Fatalf("%s returned error: %v", tt.name, err)
			}
			if dbObj.sql != tt.sql {
				t.Fatalf("%s SQL mismatch", tt.name)
			}
			if !strings.Contains(strings.ToLower(dbObj.sql), "@{chain_no}") {
				t.Fatalf("%s SQL must include scoped chain_no parameter: %s", tt.name, dbObj.sql)
			}
			want := chainNoParam{ChainNo: "chain-a"}
			if !reflect.DeepEqual(dbObj.input, want) {
				t.Fatalf("%s input = %#v, want %#v", tt.name, dbObj.input, want)
			}
		})
	}
}

type recordingExecuter struct {
	sql   string
	input any
}

func (r *recordingExecuter) Query(ctx context.Context, sql string, input any, opts ...xdb.TemplateOption) (xdb.Rows, error) {
	return nil, nil
}

func (r *recordingExecuter) Multi(ctx context.Context, sql string, input any, opts ...xdb.TemplateOption) ([]xdb.Rows, error) {
	return nil, nil
}

func (r *recordingExecuter) First(ctx context.Context, sql string, input any, opts ...xdb.TemplateOption) (xdb.Row, error) {
	return nil, nil
}

func (r *recordingExecuter) Scalar(ctx context.Context, sql string, input any, opts ...xdb.TemplateOption) (interface{}, error) {
	return nil, nil
}

func (r *recordingExecuter) Exec(ctx context.Context, sql string, input any, opts ...xdb.TemplateOption) (xdb.Result, error) {
	return nil, nil
}

func (r *recordingExecuter) QueryAs(ctx context.Context, sql string, input any, result any, opts ...xdb.TemplateOption) error {
	r.sql = sql
	r.input = input
	return nil
}

func (r *recordingExecuter) FirstAs(ctx context.Context, sql string, input any, result any, opts ...xdb.TemplateOption) error {
	return nil
}
