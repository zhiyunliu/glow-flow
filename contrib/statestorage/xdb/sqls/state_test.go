package sqls

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zhiyunliu/glue/contrib/xdb/expression"
	"github.com/zhiyunliu/glue/contrib/xdb/tpl"
	"github.com/zhiyunliu/glue/xdb"
)

func newTestTemplate() xdb.SQLTemplate {
	return tpl.NewFixed("statestorage_test", "?", tpl.NewDefaultTemplateMatcher(expression.DefaultExpressionMatchers...), nil)
}

func renderSQL(t *testing.T, query string, input map[string]any) (string, []any) {
	t.Helper()
	sql, args, err := newTestTemplate().GetSQLContext(query, input, xdb.WithExprCache(false))
	if err != nil {
		t.Fatalf("render sql: %v\n%s", err, query)
	}
	if strings.Contains(sql, "@{") || strings.Contains(sql, "&{") {
		t.Fatalf("unresolved template expression in sql:\n%s", sql)
	}
	return normalizeSpace(sql), args
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestStateSQLTemplatesResolveRequiredParameters(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		input    map[string]any
		wantArgs int
		required []string
	}{
		{
			name:     "SaveInstanceState",
			query:    SaveInstanceState,
			input:    map[string]any{"instance_id": "i1", "chain_id": "c1", "version": "v1", "status": "running", "state_data": "{}"},
			wantArgs: 10,
			required: []string{"merge dbo.flow_instance_state with (holdlock)", "when matched then", "when not matched then"},
		},
		{
			name:     "GetInstanceState",
			query:    GetInstanceState,
			input:    map[string]any{"instance_id": "i1"},
			wantArgs: 1,
			required: []string{"from dbo.flow_instance_state", "state.instance_id = ?"},
		},
		{
			name:     "DeleteInstanceState",
			query:    DeleteInstanceState,
			input:    map[string]any{"instance_id": "i1"},
			wantArgs: 1,
			required: []string{"delete from dbo.flow_instance_state", "instance_id = ?"},
		},
		{
			name:     "SaveNodeState",
			query:    SaveNodeState,
			input:    map[string]any{"instance_id": "i1", "node_id": "n1", "chain_id": "c1", "version": "v1", "status": "running", "attempts": 1, "state_data": "{}"},
			wantArgs: 14,
			required: []string{"merge dbo.flow_node_state with (holdlock)", "target.node_id = source.node_id"},
		},
		{
			name:     "GetNodeState",
			query:    GetNodeState,
			input:    map[string]any{"instance_id": "i1", "node_id": "n1"},
			wantArgs: 2,
			required: []string{"from dbo.flow_node_state", "state.instance_id = ?", "state.node_id = ?"},
		},
		{
			name:     "DeleteNodeState",
			query:    DeleteNodeState,
			input:    map[string]any{"instance_id": "i1", "node_id": "n1"},
			wantArgs: 2,
			required: []string{"delete from dbo.flow_node_state", "node_id = ?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := renderSQL(t, tt.query, tt.input)
			lower := strings.ToLower(sql)
			for _, required := range tt.required {
				if !strings.Contains(lower, required) {
					t.Fatalf("sql missing %q:\n%s", required, sql)
				}
			}
			// 固定占位符模板中，同一参数每出现一次就追加一个参数值。
			if len(args) != tt.wantArgs {
				t.Fatalf("args = %d %#v, want %d", len(args), args, tt.wantArgs)
			}
		})
	}
}

func TestListInstanceStatesDropsEmptyConditions(t *testing.T) {
	sql, args := renderSQL(t, ListInstanceStates, map[string]any{
		"chain_id": "",
		"version":  "",
		"status":   []string(nil),
		"offset":   0,
		"limit":    0,
	})
	lower := strings.ToLower(sql)
	for _, unexpected := range []string{"and state.chain_id", "and state.version", "state.status in", "fetch next"} {
		if strings.Contains(lower, unexpected) {
			t.Fatalf("sql should not contain %q:\n%s", unexpected, sql)
		}
	}
	if !strings.Contains(lower, "order by state.instance_id offset ? rows") {
		t.Fatalf("sql missing ordered offset:\n%s", sql)
	}
	if !reflect.DeepEqual(args, []any{0}) {
		t.Fatalf("args = %#v, want offset only", args)
	}
}

func TestListInstanceStatesWithLimitAppliesAllConditions(t *testing.T) {
	sql, args := renderSQL(t, ListInstanceStatesWithLimit, map[string]any{
		"chain_id": "flow-a",
		"version":  "v1",
		"status":   []string{"running", "paused"},
		"offset":   1,
		"limit":    2,
	})
	lower := strings.ToLower(sql)
	for _, required := range []string{
		"and state.chain_id=?",
		"and state.version=?",
		"and state.status in ('running','paused')",
		"offset ? rows fetch next ? rows only",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("sql missing %q:\n%s", required, sql)
		}
	}
	if !reflect.DeepEqual(args, []any{"flow-a", "v1", 1, 2}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestListNodeStatesWithLimitAppliesAllConditions(t *testing.T) {
	sql, args := renderSQL(t, ListNodeStatesWithLimit, map[string]any{
		"instance_id": "inst-001",
		"chain_id":    "flow-a",
		"node_id":     "node",
		"status":      []string{"completed"},
		"offset":      0,
		"limit":       10,
	})
	lower := strings.ToLower(sql)
	for _, required := range []string{
		"and state.instance_id=?",
		"and state.chain_id=?",
		"and state.node_id like '%'+?+'%'",
		"and state.status in ('completed')",
		"order by state.instance_id, state.node_id offset ? rows fetch next ? rows only",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("sql missing %q:\n%s", required, sql)
		}
	}
	if !reflect.DeepEqual(args, []any{"inst-001", "flow-a", "node", 0, 10}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestListNodeStatesDropsEmptyConditions(t *testing.T) {
	sql, _ := renderSQL(t, ListNodeStates, map[string]any{
		"instance_id": "",
		"chain_id":    "",
		"node_id":     "",
		"status":      []string{},
		"offset":      0,
		"limit":       0,
	})
	lower := strings.ToLower(sql)
	for _, unexpected := range []string{"and state.", " like ", " in (", "fetch next"} {
		if strings.Contains(lower, unexpected) {
			t.Fatalf("sql should not contain %q:\n%s", unexpected, sql)
		}
	}
}
