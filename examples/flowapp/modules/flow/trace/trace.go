// Package trace 记录流程执行轨迹：节点注册表包装器记录每个节点的执行情况，
// 记录器通过 context 随流程传递，自定义节点也可通过它收集流程结果。
package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/zhiyunliu/glow-flow/examples/flowapp/models"
)

// Recorder 记录一次流程执行的节点轨迹与收集的结果
type Recorder struct {
	seq     atomic.Int64
	mu      sync.Mutex
	events  []models.TraceEvent
	results []any
}

type recorderKey struct{}

func NewRecorder() *Recorder {
	return &Recorder{}
}

// WithRecorder 将记录器放入 context，流程执行时各节点从中获取
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, rec)
}

// FromContext 获取记录器；未携带时返回 nil
func FromContext(ctx context.Context) *Recorder {
	rec, _ := ctx.Value(recorderKey{}).(*Recorder)
	return rec
}

// Collect 收集一项流程结果
func (r *Recorder) Collect(data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, snapshot(data))
}

// Events 按节点开始执行的顺序返回轨迹
func (r *Recorder) Events() []models.TraceEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := append([]models.TraceEvent{}, r.events...)
	sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	return events
}

// Results 收集的流程结果
func (r *Recorder) Results() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]any{}, r.results...)
}

func (r *Recorder) add(ev models.TraceEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

// snapshot 以 JSON 形式复制数据，避免下游节点修改后影响已记录的内容
func snapshot(data any) any {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}
