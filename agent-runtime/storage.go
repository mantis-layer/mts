package agentruntime

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Storage 是 TaskRun 持久化接口（FR-007）：
// 只负责写入、读取、恢复；不暴露 SQLite 特有概念，
// 也不实现删除/保留/TTL（PRD Deferred）。
type Storage interface {
	SaveTask(ctx context.Context, task *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)

	CreateRun(ctx context.Context, run *TaskRun) error
	UpdateRun(ctx context.Context, run *TaskRun) error
	GetRun(ctx context.Context, id string) (*TaskRun, error)

	// UpdateRunIf 只在 run 当前状态为 from 时原子更新全部字段；
	// 状态已被并发修改（如外部取消）返回 (false, nil)，不覆盖。
	UpdateRunIf(ctx context.Context, run *TaskRun, from RunState) (bool, error)

	AddEvent(ctx context.Context, ev *RuntimeEvent) error
	Events(ctx context.Context, runID string) ([]RuntimeEvent, error)

	AddArtifact(ctx context.Context, a *Artifact) error
	Artifacts(ctx context.Context, runID string) ([]Artifact, error)
	AddEvidence(ctx context.Context, e *Evidence) error
	Evidence(ctx context.Context, artifactID string) ([]Evidence, error)

	// AddArtifactsEvidence 原子地写入一批 Artifact 与 Evidence（SQLite 单事务；
	// 中途失败不留孤立数据）。Runtime 步骤产出统一走此方法。
	AddArtifactsEvidence(ctx context.Context, arts []Artifact, evs []Evidence) error

	// CompareAndSetState 原子地把 run 从 from 迁移到 to；若当前状态不是 from 返回 (false, nil)。
	// 用于并发安全的状态迁移（外部 Cancel 与 Run 的状态机写不互相覆盖）。
	CompareAndSetState(ctx context.Context, runID string, from, to RunState) (bool, error)

	// SavePersona 写入或更新 Persona（FR-010）。
	// 并发修改同一 Persona → last-write-wins（v2.0 简单方案）。
	SavePersona(ctx context.Context, p *Persona) error
	// GetPersona 按 ID 读取 Persona；不存在返回 NotFoundError。
	GetPersona(ctx context.Context, id string) (*Persona, error)
	// ListPersonas 列出全部 Persona，按 UpdatedAt 倒序（最近修改在前）。
	ListPersonas(ctx context.Context) ([]Persona, error)

	Close() error
}

// MemoryStorage 是内存实现（测试与临时使用）。
type MemoryStorage struct {
	mu        sync.RWMutex
	tasks     map[string]*Task
	runs      map[string]*TaskRun
	personae  map[string]*Persona
	events    map[string][]RuntimeEvent
	artifacts map[string][]Artifact
	evidence  map[string][]Evidence
	nextID    int
}

// NewMemoryStorage 构造内存 Storage。
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		tasks:     make(map[string]*Task),
		runs:      make(map[string]*TaskRun),
		personae:  make(map[string]*Persona),
		events:    make(map[string][]RuntimeEvent),
		artifacts: make(map[string][]Artifact),
		evidence:  make(map[string][]Evidence),
	}
}

func (s *MemoryStorage) SaveTask(_ context.Context, task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return nil
}

func (s *MemoryStorage) GetTask(_ context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, &NotFoundError{Kind: "task", ID: id}
	}
	return t, nil
}

func (s *MemoryStorage) CreateRun(_ context.Context, run *TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; ok {
		return &StateError{Op: "create run", From: RunState(run.State), To: ""}
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *MemoryStorage) UpdateRun(_ context.Context, run *TaskRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; !ok {
		return &NotFoundError{Kind: "run", ID: run.ID}
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

// UpdateRunIf 内存实现：锁内校验当前状态。
func (s *MemoryStorage) UpdateRunIf(_ context.Context, run *TaskRun, from RunState) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.runs[run.ID]
	if !ok {
		return false, &NotFoundError{Kind: "run", ID: run.ID}
	}
	if cur.State != from {
		return false, nil
	}
	s.runs[run.ID] = cloneRun(run)
	return true, nil
}

func (s *MemoryStorage) GetRun(_ context.Context, id string) (*TaskRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, &NotFoundError{Kind: "run", ID: id}
	}
	return cloneRun(r), nil
}

func (s *MemoryStorage) AddEvent(_ context.Context, ev *RuntimeEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[ev.TaskRunID] = append(s.events[ev.TaskRunID], *ev)
	return nil
}

func (s *MemoryStorage) Events(_ context.Context, runID string) ([]RuntimeEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]RuntimeEvent(nil), s.events[runID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

func (s *MemoryStorage) AddArtifact(_ context.Context, a *Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[a.TaskRunID] = append(s.artifacts[a.TaskRunID], *a)
	return nil
}

func (s *MemoryStorage) Artifacts(_ context.Context, runID string) ([]Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Artifact(nil), s.artifacts[runID]...), nil
}

func (s *MemoryStorage) AddEvidence(_ context.Context, e *Evidence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[e.ArtifactID] = append(s.evidence[e.ArtifactID], *e)
	return nil
}

func (s *MemoryStorage) Evidence(_ context.Context, artifactID string) ([]Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Evidence(nil), s.evidence[artifactID]...), nil
}

// AddArtifactsEvidence 内存实现：锁内批量写入（原子）。
func (s *MemoryStorage) AddArtifactsEvidence(_ context.Context, arts []Artifact, evs []Evidence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range arts {
		s.artifacts[a.TaskRunID] = append(s.artifacts[a.TaskRunID], a)
	}
	for _, e := range evs {
		s.evidence[e.ArtifactID] = append(s.evidence[e.ArtifactID], e)
	}
	return nil
}

// CompareAndSetState 内存实现：锁内检查当前状态。
func (s *MemoryStorage) CompareAndSetState(_ context.Context, runID string, from, to RunState) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return false, &NotFoundError{Kind: "run", ID: runID}
	}
	if r.State != from {
		return false, nil
	}
	r.State = to
	r.UpdatedAt = time.Now()
	return true, nil
}

func (s *MemoryStorage) SavePersona(_ context.Context, p *Persona) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// last-write-wins：直接覆盖（含 UpdatedAt）。
	cp := *p
	s.personae[p.ID] = &cp
	return nil
}

func (s *MemoryStorage) GetPersona(_ context.Context, id string) (*Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.personae[id]
	if !ok {
		return nil, &NotFoundError{Kind: "persona", ID: id}
	}
	cp := *p
	return &cp, nil
}

func (s *MemoryStorage) ListPersonas(_ context.Context) ([]Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Persona, 0, len(s.personae))
	for _, p := range s.personae {
		out = append(out, *p)
	}
	// 按 UpdatedAt 倒序（最近修改在前）。
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *MemoryStorage) Close() error { return nil }

// NotFoundError 是查询不存在对象的结构化错误。
type NotFoundError struct {
	Kind string
	ID   string
}

func (e *NotFoundError) Error() string {
	return "agentruntime: " + e.Kind + " 不存在: " + e.ID
}
