package jsonfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	glowflow "github.com/zhiyunliu/glow-flow"
)

const basicInfraFileName = "basic.infra.json"
const reloadDelay = 100 * time.Millisecond

// NewRepository creates a new JSON file repository.
func NewRepository() glowflow.DataRepository {
	repo := &jsonFileRepository{
		dirs: repositoryDirs(),
	}
	_ = repo.reload()
	_ = repo.watch()
	return repo
}

type jsonFileRepository struct {
	mu          sync.RWMutex
	dirs        []string
	chains      []*glowflow.ChainDefinition
	basicInfras []*glowflow.BasicInfra
	loadErr     error
	watcher     *fsnotify.Watcher
	closeOnce   sync.Once
	reloadCh    chan struct{}
	done        chan struct{}
}

func (r *jsonFileRepository) LoadChainDefinitions(ctx context.Context) ([]*glowflow.ChainDefinition, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	definitions := make([]*glowflow.ChainDefinition, len(r.chains))
	copy(definitions, r.chains)
	return definitions, nil
}

func (r *jsonFileRepository) LoadChainDefinition(ctx context.Context, chainNo string) (*glowflow.ChainDefinition, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	for _, definition := range r.chains {
		if definition.ID == chainNo {
			return definition, nil
		}
	}
	return nil, fmt.Errorf("chain definition %q not found", chainNo)
}

func (r *jsonFileRepository) LoadBasicInfra(ctx context.Context, infraNo string) (*glowflow.BasicInfra, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	for _, infra := range r.basicInfras {
		if infra != nil && infra.InfraNo == infraNo {
			return infra, nil
		}
	}
	return nil, fmt.Errorf("basic infra %q not found", infraNo)
}

func (r *jsonFileRepository) Close() error {
	var err error
	r.closeOnce.Do(func() {
		if r.watcher != nil {
			err = r.watcher.Close()
		}
		if r.done != nil {
			<-r.done
		}
	})
	return err
}

func (r *jsonFileRepository) reload() error {
	chains, infras, err := r.readAll()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.loadErr = err
		return err
	}
	r.chains = chains
	r.basicInfras = infras
	r.loadErr = nil
	return nil
}

func (r *jsonFileRepository) readAll() ([]*glowflow.ChainDefinition, []*glowflow.BasicInfra, error) {
	chainFiles, err := r.chainDefinitionFiles()
	if err != nil {
		return nil, nil, err
	}
	definitions := make([]*glowflow.ChainDefinition, 0, len(chainFiles))
	for _, file := range chainFiles {
		definition := &glowflow.ChainDefinition{}
		if err := readJSONFile(file, definition); err != nil {
			return nil, nil, err
		}
		definitions = append(definitions, definition)
	}

	infraFiles, err := r.basicInfraFiles()
	if err != nil {
		return nil, nil, err
	}
	infras := make([]*glowflow.BasicInfra, 0)
	for _, file := range infraFiles {
		current := make([]*glowflow.BasicInfra, 0)
		if err := readJSONFile(file, &current); err != nil {
			return nil, nil, err
		}
		infras = append(infras, current...)
	}
	return definitions, infras, nil
}

func (r *jsonFileRepository) watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	r.watcher = watcher
	r.reloadCh = make(chan struct{}, 1)
	r.done = make(chan struct{})
	for _, dir := range r.dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = watcher.Close()
			return err
		}
		if err := watcher.Add(dir); err != nil {
			_ = watcher.Close()
			return err
		}
	}
	go r.runWatcher()
	return nil
}

func (r *jsonFileRepository) runWatcher() {
	defer close(r.done)
	var timer *time.Timer
	var timerCh <-chan time.Time
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			if shouldReload(event) {
				if timer == nil {
					timer = time.NewTimer(reloadDelay)
					timerCh = timer.C
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(reloadDelay)
				}
			}
		case _, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.scheduleReload()
		case <-r.reloadCh:
			_ = r.reload()
		case <-timerCh:
			_ = r.reload()
			timerCh = nil
		}
	}
}

func (r *jsonFileRepository) scheduleReload() {
	select {
	case r.reloadCh <- struct{}{}:
	default:
	}
}

func (r *jsonFileRepository) chainDefinitionFiles() ([]string, error) {
	files := make([]string, 0)
	for _, dir := range r.dirs {
		entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(entries)
		for _, entry := range entries {
			if strings.EqualFold(filepath.Base(entry), basicInfraFileName) {
				continue
			}
			files = append(files, entry)
		}
	}
	return files, nil
}

func (r *jsonFileRepository) basicInfraFiles() ([]string, error) {
	files := make([]string, 0)
	for _, dir := range r.dirs {
		file := filepath.Join(dir, basicInfraFileName)
		if _, err := os.Stat(file); err == nil {
			files = append(files, file)
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return files, nil
}

func shouldReload(event fsnotify.Event) bool {
	if !strings.EqualFold(filepath.Ext(event.Name), ".json") {
		return false
	}
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)
}

func repositoryDirs() []string {
	return []string{
		filepath.Join(".", "glowflow"),
		filepath.Join("..", "etc", "glowflow"),
	}
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
