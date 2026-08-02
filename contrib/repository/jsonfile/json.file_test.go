package jsonfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	glowflow "github.com/zhiyunliu/glow-flow"
)

func TestJSONFileRepositoryLoadsChainDefinitionsFromConfiguredDirectories(t *testing.T) {
	workDir := prepareJSONFileRepositoryDirs(t)
	writeJSONFile(t, filepath.Join(workDir, "glowflow", "chain-a.json"), glowflow.ChainDefinition{
		ID:      "chain-a",
		Version: "v1",
		Metadata: glowflow.ChainMetadata{
			ID:     "chain-a",
			Name:   "Chain A",
			Status: 1,
		},
	})
	writeJSONFile(t, filepath.Join(workDir, "..", "etc", "glowflow", "chain-b.json"), glowflow.ChainDefinition{
		ID:      "chain-b",
		Version: "v2",
		Metadata: glowflow.ChainMetadata{
			ID:     "chain-b",
			Name:   "Chain B",
			Status: 1,
		},
	})
	writeJSONFile(t, filepath.Join(workDir, "glowflow", "basic.infra.json"), []glowflow.BasicInfra{{InfraNo: "infra-a"}})

	definitions, err := newJSONFileRepository(t).LoadChainDefinitions(context.Background())
	if err != nil {
		t.Fatalf("LoadChainDefinitions returned error: %v", err)
	}
	gotIDs := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		gotIDs = append(gotIDs, definition.ID)
	}
	wantIDs := []string{"chain-a", "chain-b"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("chain definition ids = %#v, want %#v", gotIDs, wantIDs)
	}
}

func TestJSONFileRepositoryLoadChainDefinitionReturnsRequestedChain(t *testing.T) {
	workDir := prepareJSONFileRepositoryDirs(t)
	writeJSONFile(t, filepath.Join(workDir, "glowflow", "chain-a.json"), glowflow.ChainDefinition{ID: "chain-a", Version: "v1"})
	writeJSONFile(t, filepath.Join(workDir, "..", "etc", "glowflow", "chain-b.json"), glowflow.ChainDefinition{ID: "chain-b", Version: "v2"})

	definition, err := newJSONFileRepository(t).LoadChainDefinition(context.Background(), "chain-b")
	if err != nil {
		t.Fatalf("LoadChainDefinition returned error: %v", err)
	}
	if definition.ID != "chain-b" || definition.Version != "v2" {
		t.Fatalf("unexpected chain definition: %+v", definition)
	}
}

func TestJSONFileRepositoryLoadBasicInfraReturnsRequestedInfra(t *testing.T) {
	workDir := prepareJSONFileRepositoryDirs(t)
	writeJSONFile(t, filepath.Join(workDir, "..", "etc", "glowflow", "basic.infra.json"), []glowflow.BasicInfra{
		{
			InfraNo: "redis-main",
			Name:    "Redis Main",
			Type:    "redis",
			Desc:    "cache",
			ExtParams: map[string]any{
				"addr": "127.0.0.1:6379",
			},
		},
		{InfraNo: "mssql-main", Type: "database"},
	})

	infra, err := newJSONFileRepository(t).LoadBasicInfra(context.Background(), "redis-main")
	if err != nil {
		t.Fatalf("LoadBasicInfra returned error: %v", err)
	}
	if infra.InfraNo != "redis-main" || infra.Name != "Redis Main" || infra.Type != "redis" || infra.Desc != "cache" {
		t.Fatalf("unexpected basic infra: %+v", infra)
	}
	if infra.ExtParams["addr"] != "127.0.0.1:6379" {
		t.Fatalf("unexpected basic infra extparams: %#v", infra.ExtParams)
	}
}

func TestJSONFileRepositoryReturnsNotFound(t *testing.T) {
	prepareJSONFileRepositoryDirs(t)
	repo := newJSONFileRepository(t)

	if _, err := repo.LoadChainDefinition(context.Background(), "missing-chain"); err == nil {
		t.Fatalf("expected missing chain definition to return an error")
	}
	if _, err := repo.LoadBasicInfra(context.Background(), "missing-infra"); err == nil {
		t.Fatalf("expected missing basic infra to return an error")
	}
}

func TestJSONFileRepositoryCachesAndRefreshesWhenFilesChange(t *testing.T) {
	workDir := prepareJSONFileRepositoryDirs(t)
	chainFile := filepath.Join(workDir, "glowflow", "chain-a.json")
	infraFile := filepath.Join(workDir, "glowflow", "basic.infra.json")
	writeJSONFile(t, chainFile, glowflow.ChainDefinition{ID: "chain-a", Version: "v1"})
	writeJSONFile(t, infraFile, []glowflow.BasicInfra{{InfraNo: "redis-main", Type: "redis-v1"}})
	repo := newJSONFileRepository(t)

	definition, err := repo.LoadChainDefinition(context.Background(), "chain-a")
	if err != nil {
		t.Fatalf("LoadChainDefinition returned error: %v", err)
	}
	if definition.Version != "v1" {
		t.Fatalf("initial chain version = %q, want v1", definition.Version)
	}
	infra, err := repo.LoadBasicInfra(context.Background(), "redis-main")
	if err != nil {
		t.Fatalf("LoadBasicInfra returned error: %v", err)
	}
	if infra.Type != "redis-v1" {
		t.Fatalf("initial infra type = %q, want redis-v1", infra.Type)
	}

	writeJSONFile(t, chainFile, glowflow.ChainDefinition{ID: "chain-a", Version: "v2"})
	writeJSONFile(t, infraFile, []glowflow.BasicInfra{{InfraNo: "redis-main", Type: "redis-v2"}})

	waitForJSONFileRepository(t, func() bool {
		definition, err := repo.LoadChainDefinition(context.Background(), "chain-a")
		if err != nil || definition.Version != "v2" {
			return false
		}
		infra, err := repo.LoadBasicInfra(context.Background(), "redis-main")
		return err == nil && infra.Type == "redis-v2"
	})
}

func TestJSONFileRepositoryKeepsInMemoryCacheAfterSourceFileRemoved(t *testing.T) {
	workDir := prepareJSONFileRepositoryDirs(t)
	chainFile := filepath.Join(workDir, "glowflow", "chain-a.json")
	infraFile := filepath.Join(workDir, "glowflow", "basic.infra.json")
	writeJSONFile(t, chainFile, glowflow.ChainDefinition{ID: "chain-a", Version: "v1"})
	writeJSONFile(t, infraFile, []glowflow.BasicInfra{{InfraNo: "redis-main", Type: "redis-v1"}})
	repo := NewRepository()
	closer, ok := repo.(interface{ Close() error })
	if !ok {
		t.Fatalf("jsonfile repository must expose Close to stop directory watcher")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if err := os.Remove(chainFile); err != nil {
		t.Fatalf("remove %s: %v", chainFile, err)
	}
	if err := os.Remove(infraFile); err != nil {
		t.Fatalf("remove %s: %v", infraFile, err)
	}

	definition, err := repo.LoadChainDefinition(context.Background(), "chain-a")
	if err != nil {
		t.Fatalf("LoadChainDefinition should read cached value after source file removal: %v", err)
	}
	if definition.Version != "v1" {
		t.Fatalf("cached chain version = %q, want v1", definition.Version)
	}
	infra, err := repo.LoadBasicInfra(context.Background(), "redis-main")
	if err != nil {
		t.Fatalf("LoadBasicInfra should read cached value after source file removal: %v", err)
	}
	if infra.Type != "redis-v1" {
		t.Fatalf("cached infra type = %q, want redis-v1", infra.Type)
	}
}

func prepareJSONFileRepositoryDirs(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()
	workDir := filepath.Join(rootDir, "app")
	for _, dir := range []string{
		filepath.Join(workDir, "glowflow"),
		filepath.Join(rootDir, "etc", "glowflow"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("chdir %s: %v", workDir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	return workDir
}

func newJSONFileRepository(t *testing.T) glowflow.DataRepository {
	t.Helper()
	repo := NewRepository()
	closer, ok := repo.(interface{ Close() error })
	if ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	return repo
}

func waitForJSONFileRepository(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("condition was not met before timeout")
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
