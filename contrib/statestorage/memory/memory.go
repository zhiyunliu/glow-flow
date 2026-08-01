package memory

import (
	"sync"

	glowflow "github.com/zhiyunliu/glow-flow"
)

func NewStateStorage() glowflow.StateStorage {
	return &memoryStateStorage{}
}

type memoryStateStorage struct {
	mu sync.RWMutex
}
