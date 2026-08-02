package xdb

import (
	"context"
	"database/sql"
	"reflect"
	"strconv"
	"testing"

	glowflow "github.com/zhiyunliu/glow-flow"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/models"
	"github.com/zhiyunliu/glow-flow/contrib/repository/xdb/sqls"
	gluexdb "github.com/zhiyunliu/glue/xdb"
)

func TestXDBBuildChainDefinitionsSelectsLatestActiveVersionAndAssemblesTopology(t *testing.T) {
	repo := &xdbRepository{}
	chains := []*models.ChainDefinition{
		xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10000),
			"Name":      "Chain A v1",
			"Status":    1,
		}),
		xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10001),
			"Name":      "Chain A v2",
			"Status":    1,
			"ExtParams": `{"priority":"new","retries":3}`,
			"Layout":    `{"icon":"flow","X":11,"Y":12,"W":3,"H":4}`,
		}),
		xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10002),
			"Name":      "Chain A disabled newest",
			"Status":    0,
		}),
		xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-b",
			"VersionID": int64(20000),
			"Name":      "Chain B",
			"Status":    1,
		}),
		xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-c",
			"VersionID": int64(30000),
			"Name":      "Chain C disabled",
			"Status":    0,
		}),
	}
	nodes := []*models.NodeDefinition{
		xdbModel[models.NodeDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10000),
			"NodeDefNo": "node-old",
			"Name":      "Old Node",
			"Type":      "old",
		}),
		xdbModel[models.NodeDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10001),
			"NodeDefNo": "node-start",
			"Name":      "Start Node",
			"Type":      "start",
			"ExtParams": `{"timeout":30}`,
			"Layout":    `{"icon":"play","X":1,"Y":2,"W":3,"H":4}`,
		}),
		xdbModel[models.NodeDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10001),
			"NodeDefNo": "node-end",
			"Name":      "End Node",
			"Type":      "end",
		}),
		xdbModel[models.NodeDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10002),
			"NodeDefNo": "node-disabled",
			"Name":      "Disabled Node",
			"Type":      "disabled",
		}),
		xdbModel[models.NodeDefinition](t, map[string]any{
			"ChainNo":   "chain-b",
			"VersionID": int64(20000),
			"NodeDefNo": "node-b",
			"Name":      "Chain B Node",
			"Type":      "task",
		}),
	}
	connections := []*models.ConnectionDefinition{
		xdbModel[models.ConnectionDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10000),
			"FromID":    "node-old",
			"ToID":      "node-start",
			"Type":      "old",
		}),
		xdbModel[models.ConnectionDefinition](t, map[string]any{
			"ChainNo":   "chain-a",
			"VersionID": int64(10001),
			"FromID":    "node-start",
			"ToID":      "node-end",
			"Type":      "success",
			"Remark":    "continue",
		}),
		xdbModel[models.ConnectionDefinition](t, map[string]any{
			"ChainNo":   "chain-b",
			"VersionID": int64(20000),
			"FromID":    "node-b",
			"ToID":      "node-b",
			"Type":      "self",
		}),
	}

	definitions, err := repo.buildChainDefinition(context.Background(), chains, nodes, connections)
	if err != nil {
		t.Fatalf("buildChainDefinition returned error: %v", err)
	}
	if len(definitions) != 2 {
		t.Fatalf("expected only active chains with latest active versions, got %d", len(definitions))
	}

	chainA := findXDBChain(t, definitions, "chain-a")
	if chainA.Version != "10001" {
		t.Fatalf("expected latest active version 10001, got %q", chainA.Version)
	}
	if chainA.Metadata.Name != "Chain A v2" || chainA.Metadata.Status != 1 {
		t.Fatalf("unexpected chain metadata: %+v", chainA.Metadata)
	}
	if chainA.Metadata.ExtParams["priority"] != "new" || chainA.Metadata.ExtParams["retries"] != float64(3) {
		t.Fatalf("unexpected chain extparams: %#v", chainA.Metadata.ExtParams)
	}
	if chainA.Metadata.Layout != (glowflow.Layout{Icon: "flow", X: 11, Y: 12, W: 3, H: 4}) {
		t.Fatalf("unexpected chain layout: %+v", chainA.Metadata.Layout)
	}
	if len(chainA.Nodes) != 2 {
		t.Fatalf("expected two nodes for selected version only, got %d", len(chainA.Nodes))
	}
	start := findXDBNode(t, chainA.Nodes, "node-start")
	if start.Type != "start" || start.Name != "Start Node" {
		t.Fatalf("unexpected joined node definition: %+v", start)
	}
	if start.ExtParams["timeout"] != float64(30) {
		t.Fatalf("unexpected node extparams: %#v", start.ExtParams)
	}
	if start.Layout != (glowflow.Layout{Icon: "play", X: 1, Y: 2, W: 3, H: 4}) {
		t.Fatalf("unexpected node layout: %+v", start.Layout)
	}
	if len(chainA.Connections) != 1 {
		t.Fatalf("expected one connection for selected version only, got %d", len(chainA.Connections))
	}
	if chainA.Connections[0] != (glowflow.ConnectionDefinition{FromID: "node-start", ToID: "node-end", Type: "success", Remark: "continue"}) {
		t.Fatalf("unexpected connection: %+v", chainA.Connections[0])
	}

	chainB := findXDBChain(t, definitions, "chain-b")
	if chainB.Version != "20000" || len(chainB.Nodes) != 1 || len(chainB.Connections) != 1 {
		t.Fatalf("unexpected scoped chain-b output: %+v", chainB)
	}
}

func TestXDBLoadChainDefinitionReturnsRequestedLatestActiveChain(t *testing.T) {
	dbObj := &repositoryExecuter{
		chains: []*models.ChainDefinition{
			xdbModel[models.ChainDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10000),
				"Name":      "Chain A old",
				"Status":    1,
			}),
			xdbModel[models.ChainDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10001),
				"Name":      "Chain A latest",
				"Status":    1,
			}),
			xdbModel[models.ChainDefinition](t, map[string]any{
				"ChainNo":   "chain-b",
				"VersionID": int64(20000),
				"Name":      "Chain B",
				"Status":    1,
			}),
		},
		nodes: []*models.NodeDefinition{
			xdbModel[models.NodeDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10001),
				"NodeDefNo": "node-a",
				"Name":      "Node A",
				"Type":      "start",
			}),
			xdbModel[models.NodeDefinition](t, map[string]any{
				"ChainNo":   "chain-b",
				"VersionID": int64(20000),
				"NodeDefNo": "node-b",
				"Name":      "Node B",
				"Type":      "task",
			}),
		},
		connections: []*models.ConnectionDefinition{
			xdbModel[models.ConnectionDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10001),
				"ConnID":    int64(1),
				"FromID":    "node-a",
				"ToID":      "node-a",
				"Type":      "self",
			}),
		},
	}
	original := dbResolver
	dbResolver = func(name string) gluexdb.Executer {
		if name != "test-db" {
			t.Fatalf("db resolver name = %q, want test-db", name)
		}
		return dbObj
	}
	t.Cleanup(func() { dbResolver = original })

	definition, err := (&xdbRepository{dbConnName: "test-db"}).LoadChainDefinition(context.Background(), "chain-a")
	if err != nil {
		t.Fatalf("LoadChainDefinition returned error: %v", err)
	}
	if definition.ID != "chain-a" || definition.Version != "10001" || definition.Metadata.Name != "Chain A latest" {
		t.Fatalf("unexpected chain definition: %+v", definition)
	}
	if len(definition.Nodes) != 1 || definition.Nodes[0].ID != "node-a" {
		t.Fatalf("expected scoped node for chain-a only, got %+v", definition.Nodes)
	}
	if len(definition.Connections) != 1 || definition.Connections[0].FromID != "node-a" {
		t.Fatalf("expected scoped connection for chain-a only, got %+v", definition.Connections)
	}
	if got := dbObj.chainNoInputs; !reflect.DeepEqual(got, []string{"chain-a", "chain-a", "chain-a"}) {
		t.Fatalf("chain_no inputs = %#v, want three scoped chain-a calls", got)
	}
}

func TestXDBLoadBasicInfraReturnsRequestedInfra(t *testing.T) {
	dbObj := &repositoryExecuter{
		basicInfras: []*models.BasicInfra{
			xdbModel[models.BasicInfra](t, map[string]any{
				"InfraNo":   "infra-a",
				"InfraType": "database",
				"ExtParams": `{"dsn":"sqlserver","max_open":10}`,
				"Desc":      "SQL Server infra",
			}),
			xdbModel[models.BasicInfra](t, map[string]any{
				"InfraNo":   "infra-b",
				"InfraType": "redis",
			}),
		},
	}
	original := dbResolver
	dbResolver = func(name string) gluexdb.Executer {
		if name != "test-db" {
			t.Fatalf("db resolver name = %q, want test-db", name)
		}
		return dbObj
	}
	t.Cleanup(func() { dbResolver = original })

	infra, err := (&xdbRepository{dbConnName: "test-db"}).LoadBasicInfra(context.Background(), "infra-a")
	if err != nil {
		t.Fatalf("LoadBasicInfra returned error: %v", err)
	}
	if infra.InfraNo != "infra-a" || infra.Type != "database" || infra.Name != "infra-a" || infra.Desc != "SQL Server infra" {
		t.Fatalf("unexpected basic infra: %+v", infra)
	}
	if infra.ExtParams["dsn"] != "sqlserver" || infra.ExtParams["max_open"] != float64(10) {
		t.Fatalf("unexpected basic infra extparams: %#v", infra.ExtParams)
	}
	if got := dbObj.infraNoInputs; !reflect.DeepEqual(got, []string{"infra-a"}) {
		t.Fatalf("infra_no inputs = %#v, want one scoped infra-a call", got)
	}
}

func TestXDBLoadBasicInfraReturnsNotFound(t *testing.T) {
	dbObj := &repositoryExecuter{}
	original := dbResolver
	dbResolver = func(name string) gluexdb.Executer { return dbObj }
	t.Cleanup(func() { dbResolver = original })

	_, err := (&xdbRepository{dbConnName: "test-db"}).LoadBasicInfra(context.Background(), "missing-infra")
	if err == nil {
		t.Fatalf("expected missing basic infra to return an error")
	}
}

func TestXDBLoadBasicInfraReturnsErrorForMalformedExtParams(t *testing.T) {
	dbObj := &repositoryExecuter{
		basicInfras: []*models.BasicInfra{xdbModel[models.BasicInfra](t, map[string]any{
			"InfraNo":   "infra-json",
			"InfraType": "database",
			"ExtParams": `{`,
		})},
	}
	original := dbResolver
	dbResolver = func(name string) gluexdb.Executer { return dbObj }
	t.Cleanup(func() { dbResolver = original })

	_, err := (&xdbRepository{dbConnName: "test-db"}).LoadBasicInfra(context.Background(), "infra-json")
	if err == nil {
		t.Fatalf("expected malformed basic infra extparams JSON to return an error")
	}
}

func TestXDBBuildChainDefinitionsReturnsErrorForMalformedPersistedJSON(t *testing.T) {
	repo := &xdbRepository{}
	tests := []struct {
		name       string
		chainField map[string]any
		nodeField  map[string]any
	}{
		{
			name:       "chain extparams",
			chainField: map[string]any{"ExtParams": `{`},
		},
		{
			name:       "chain layout",
			chainField: map[string]any{"Layout": `{`},
		},
		{
			name:      "node extparams",
			nodeField: map[string]any{"ExtParams": `{`},
		},
		{
			name:      "node layout",
			nodeField: map[string]any{"Layout": `{`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chainFields := map[string]any{
				"ChainNo":   "chain-json",
				"VersionID": int64(10000),
				"Name":      "JSON Chain",
				"Status":    1,
			}
			for key, value := range tt.chainField {
				chainFields[key] = value
			}
			nodeFields := map[string]any{
				"ChainNo":   "chain-json",
				"VersionID": int64(10000),
				"NodeDefNo": "node-json",
				"Name":      "JSON Node",
				"Type":      "task",
			}
			for key, value := range tt.nodeField {
				nodeFields[key] = value
			}

			_, err := repo.buildChainDefinition(
				context.Background(),
				[]*models.ChainDefinition{xdbModel[models.ChainDefinition](t, chainFields)},
				[]*models.NodeDefinition{xdbModel[models.NodeDefinition](t, nodeFields)},
				nil,
			)
			if err == nil {
				t.Fatalf("expected malformed %s JSON to return an error", tt.name)
			}
		})
	}
}

func TestXDBBuildChainDefinitionsHandlesEmptyInputs(t *testing.T) {
	repo := &xdbRepository{}
	definitions, err := repo.buildChainDefinition(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("buildChainDefinition returned error for empty inputs: %v", err)
	}
	if len(definitions) != 0 {
		t.Fatalf("expected no definitions for empty inputs, got %d", len(definitions))
	}
}

func TestXDBBuildChainDefinitionsSortsOutOfOrderRowsAndHandlesEmptyJSON(t *testing.T) {
	repo := &xdbRepository{}
	definitions, err := repo.buildChainDefinition(
		context.Background(),
		[]*models.ChainDefinition{
			xdbModel[models.ChainDefinition](t, map[string]any{
				"ChainNo":   "chain-b",
				"VersionID": int64(20000),
				"Name":      "Chain B",
				"Status":    1,
				"ExtParams": "",
				"Layout":    "null",
			}),
			xdbModel[models.ChainDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10000),
				"Name":      "Chain A",
				"Status":    1,
				"ExtParams": "null",
				"Layout":    "",
			}),
		},
		[]*models.NodeDefinition{
			xdbModel[models.NodeDefinition](t, map[string]any{
				"ChainNo":          "chain-a",
				"VersionID":        int64(10000),
				"NodeDefNo":        "node-b",
				"Name":             "Node B",
				"Type":             "task",
				"DefinitionParams": `{"source":"definition"}`,
				"LayoutDefinition": `{"icon":"default","X":9}`,
			}),
			xdbModel[models.NodeDefinition](t, map[string]any{
				"ChainNo":          "chain-a",
				"VersionID":        int64(10000),
				"NodeDefNo":        "node-a",
				"Name":             "Node A",
				"Type":             "start",
				"ExtParams":        `{"source":"node"}`,
				"Layout":           `{"icon":"node","X":1}`,
				"DefinitionParams": `{"source":"definition"}`,
				"LayoutDefinition": `{"icon":"default","X":9}`,
			}),
		},
		[]*models.ConnectionDefinition{
			xdbModel[models.ConnectionDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10000),
				"ConnID":    int64(2),
				"FromID":    "node-b",
				"ToID":      "node-a",
				"Type":      "fallback",
			}),
			xdbModel[models.ConnectionDefinition](t, map[string]any{
				"ChainNo":   "chain-a",
				"VersionID": int64(10000),
				"ConnID":    int64(1),
				"FromID":    "node-a",
				"ToID":      "node-b",
				"Type":      "success",
			}),
		},
	)
	if err != nil {
		t.Fatalf("buildChainDefinition returned error: %v", err)
	}
	if len(definitions) != 2 {
		t.Fatalf("expected two active definitions, got %d", len(definitions))
	}
	if definitions[0].ID != "chain-a" || definitions[1].ID != "chain-b" {
		t.Fatalf("expected definitions sorted by chain_no, got %+v", definitions)
	}
	chainA := definitions[0]
	if chainA.Metadata.ExtParams != nil || chainA.Metadata.Layout != (glowflow.Layout{}) {
		t.Fatalf("expected null/empty chain JSON to decode as nil and zero layout, got %+v", chainA.Metadata)
	}
	if chainA.Nodes[0].ID != "node-a" || chainA.Nodes[1].ID != "node-b" {
		t.Fatalf("expected nodes sorted by node definition id, got %+v", chainA.Nodes)
	}
	if chainA.Nodes[0].ExtParams["source"] != "node" || chainA.Nodes[0].Layout.Icon != "node" {
		t.Fatalf("expected node JSON to override definition JSON, got %+v", chainA.Nodes[0])
	}
	if chainA.Nodes[1].ExtParams["source"] != "definition" || chainA.Nodes[1].Layout.Icon != "default" {
		t.Fatalf("expected definition JSON fallback, got %+v", chainA.Nodes[1])
	}
	if chainA.Connections[0].Type != "success" || chainA.Connections[1].Type != "fallback" {
		t.Fatalf("expected connections sorted by persisted order, got %+v", chainA.Connections)
	}
}

func TestXDBBuildChainDefinitionReturnsNoDefinitionsWithoutActiveVersion(t *testing.T) {
	repo := &xdbRepository{}
	definitions, err := repo.buildChainDefinition(
		context.Background(),
		[]*models.ChainDefinition{xdbModel[models.ChainDefinition](t, map[string]any{
			"ChainNo":   "chain-disabled",
			"VersionID": int64(10000),
			"Name":      "Disabled Chain",
			"Status":    0,
		})},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("buildChainDefinition returned error: %v", err)
	}
	if len(definitions) != 0 {
		t.Fatalf("expected no definitions without active versions, got %d", len(definitions))
	}
}

func xdbModel[T any](t *testing.T, fields map[string]any) *T {
	t.Helper()
	row := new(T)
	value := reflect.ValueOf(row).Elem()
	for name, raw := range fields {
		field := value.FieldByName(name)
		if !field.IsValid() {
			t.Fatalf("%T missing field %s required by xdb contract", row, name)
		}
		setXDBField(t, name, field, raw)
	}
	return row
}

func setXDBField(t *testing.T, name string, field reflect.Value, raw any) {
	t.Helper()
	if !field.CanSet() {
		t.Fatalf("field %s cannot be set", name)
	}
	if raw == nil {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	if field.Type() == reflect.TypeOf(sql.NullString{}) {
		field.Set(reflect.ValueOf(sql.NullString{String: raw.(string), Valid: true}))
		return
	}
	if field.Kind() == reflect.Pointer {
		ptr := reflect.New(field.Type().Elem())
		setXDBField(t, name, ptr.Elem(), raw)
		field.Set(ptr)
		return
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw.(string))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(asXDBInt64(t, name, raw))
	default:
		t.Fatalf("field %s has unsupported type %s", name, field.Type())
	}
}

func asXDBInt64(t *testing.T, name string, raw any) int64 {
	t.Helper()
	switch value := raw.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			t.Fatalf("field %s cannot parse int64 from %q: %v", name, value, err)
		}
		return parsed
	default:
		t.Fatalf("field %s expected int-compatible value, got %T", name, raw)
	}
	return 0
}

func findXDBChain(t *testing.T, definitions []*glowflow.ChainDefinition, chainNo string) *glowflow.ChainDefinition {
	t.Helper()
	for _, definition := range definitions {
		if definition.ID == chainNo {
			return definition
		}
	}
	t.Fatalf("chain %s not found in %+v", chainNo, definitions)
	return nil
}

func findXDBNode(t *testing.T, nodes []glowflow.NodeDefinition, nodeID string) glowflow.NodeDefinition {
	t.Helper()
	for _, node := range nodes {
		if node.ID == nodeID {
			return node
		}
	}
	t.Fatalf("node %s not found in %+v", nodeID, nodes)
	return glowflow.NodeDefinition{}
}

type repositoryExecuter struct {
	chains        []*models.ChainDefinition
	nodes         []*models.NodeDefinition
	connections   []*models.ConnectionDefinition
	basicInfras   []*models.BasicInfra
	chainNoInputs []string
	infraNoInputs []string
}

func (r *repositoryExecuter) Query(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Rows, error) {
	return nil, nil
}

func (r *repositoryExecuter) Multi(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) ([]gluexdb.Rows, error) {
	return nil, nil
}

func (r *repositoryExecuter) First(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Row, error) {
	return nil, nil
}

func (r *repositoryExecuter) Scalar(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (interface{}, error) {
	return nil, nil
}

func (r *repositoryExecuter) Exec(ctx context.Context, sql string, input any, opts ...gluexdb.TemplateOption) (gluexdb.Result, error) {
	return nil, nil
}

func (r *repositoryExecuter) QueryAs(ctx context.Context, query string, input any, result any, opts ...gluexdb.TemplateOption) error {
	chainNo := readChainNoInput(input)
	if chainNo != "" {
		r.chainNoInputs = append(r.chainNoInputs, chainNo)
	}
	infraNo := readInfraNoInput(input)
	if infraNo != "" {
		r.infraNoInputs = append(r.infraNoInputs, infraNo)
	}
	switch query {
	case sqls.LoadChainDefinition:
		*result.(*[]*models.ChainDefinition) = filterXDBChains(r.chains, chainNo)
	case sqls.LoadChainNodesByChainNo:
		*result.(*[]*models.NodeDefinition) = filterXDBNodes(r.nodes, chainNo)
	case sqls.LoadChainConnectionsByChainNo:
		*result.(*[]*models.ConnectionDefinition) = filterXDBConnections(r.connections, chainNo)
	default:
		panic("unexpected query")
	}
	return nil
}

func readInfraNoInput(input any) string {
	value := reflect.ValueOf(input)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	field := value.FieldByName("InfraNo")
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func (r *repositoryExecuter) FirstAs(ctx context.Context, sql string, input any, result any, opts ...gluexdb.TemplateOption) error {
	infraNo := readInfraNoInput(input)
	if infraNo != "" {
		r.infraNoInputs = append(r.infraNoInputs, infraNo)
	}
	switch sql {
	case sqls.LoadBasicInfra:
		*result.(*models.BasicInfra) = *filterXDBBasicInfra(r.basicInfras, infraNo)
	default:
		panic("unexpected query")
	}
	return nil
}

func readChainNoInput(input any) string {
	value := reflect.ValueOf(input)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return ""
	}
	field := value.FieldByName("ChainNo")
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func filterXDBChains(chains []*models.ChainDefinition, chainNo string) []*models.ChainDefinition {
	filtered := make([]*models.ChainDefinition, 0)
	for _, chain := range chains {
		if chain.ChainNo == chainNo {
			filtered = append(filtered, chain)
		}
	}
	return filtered
}

func filterXDBNodes(nodes []*models.NodeDefinition, chainNo string) []*models.NodeDefinition {
	filtered := make([]*models.NodeDefinition, 0)
	for _, node := range nodes {
		if node.ChainNo == chainNo {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func filterXDBConnections(connections []*models.ConnectionDefinition, chainNo string) []*models.ConnectionDefinition {
	filtered := make([]*models.ConnectionDefinition, 0)
	for _, connection := range connections {
		if connection.ChainNo == chainNo {
			filtered = append(filtered, connection)
		}
	}
	return filtered
}

func filterXDBBasicInfra(infras []*models.BasicInfra, infraNo string) *models.BasicInfra {
	for _, infra := range infras {
		if infra.InfraNo == infraNo {
			return infra
		}
	}
	return &models.BasicInfra{}
}
