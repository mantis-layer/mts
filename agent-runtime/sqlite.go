package agentruntime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite driver（无 CGO）
)

// SQLiteStorage 是本地持久化实现（FR-007），接口不暴露 SQLite 特有概念。
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage 打开（必要时创建）SQLite 数据库文件并初始化 schema。
func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	if path == "" {
		return nil, fmt.Errorf("agentruntime: sqlite path 不能为空")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("agentruntime: 创建目录: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: 打开 sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("agentruntime: sqlite ping: %w", err)
	}
	s := &SQLiteStorage{db: db}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func dir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func (s *SQLiteStorage) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY, name TEXT, pattern TEXT, input TEXT, created_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY, task_id TEXT, pattern TEXT, state TEXT, iterations INTEGER,
			tool_calls INTEGER, usage_json TEXT, summary TEXT, input TEXT, result_json TEXT,
			error TEXT, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY, run_id TEXT, kind TEXT, data_json TEXT, timestamp TEXT)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY, run_id TEXT, name TEXT, type TEXT, content TEXT, created_at TEXT)`,
		`CREATE TABLE IF NOT EXISTS evidence (
			artifact_id TEXT, source TEXT, quote TEXT)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_run ON artifacts(run_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("agentruntime: sqlite schema: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStorage) SaveTask(ctx context.Context, task *Task) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO tasks (id, name, pattern, input, created_at) VALUES (?,?,?,?,?)`,
		task.ID, task.Name, task.Pattern, task.Input, task.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("agentruntime: save task: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetTask(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, pattern, input, created_at FROM tasks WHERE id = ?`, id)
	var t Task
	var createdAt string
	if err := row.Scan(&t.ID, &t.Name, &t.Pattern, &t.Input, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, &NotFoundError{Kind: "task", ID: id}
		}
		return nil, fmt.Errorf("agentruntime: get task: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &t, nil
}

func (s *SQLiteStorage) CreateRun(ctx context.Context, run *TaskRun) error {
	return s.insertRun(ctx, run)
}

func (s *SQLiteStorage) UpdateRun(ctx context.Context, run *TaskRun) error {
	return s.insertRun(ctx, run) // INSERT OR REPLACE 保证幂等更新
}

func (s *SQLiteStorage) insertRun(ctx context.Context, run *TaskRun) error {
	usageJSON, err := json.Marshal(run.Usage)
	if err != nil {
		return fmt.Errorf("agentruntime: marshal usage: %w", err)
	}
	var resultJSON []byte
	if run.Result != nil {
		resultJSON, err = json.Marshal(run.Result)
		if err != nil {
			return fmt.Errorf("agentruntime: marshal result: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO runs (id, task_id, pattern, state, iterations, tool_calls, usage_json, summary, input, result_json, error, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.TaskID, run.Pattern, string(run.State), run.Iterations, run.ToolCalls,
		string(usageJSON), run.Summary, run.Input, nullable(resultJSON), run.Error,
		run.CreatedAt.Format(time.RFC3339Nano), run.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("agentruntime: save run: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) GetRun(ctx context.Context, id string) (*TaskRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, task_id, pattern, state, iterations, tool_calls, usage_json, summary, input, result_json, error, created_at, updated_at FROM runs WHERE id = ?`, id)
	var r TaskRun
	var usageJSON, resultJSON, createdAt, updatedAt sql.NullString
	if err := row.Scan(&r.ID, &r.TaskID, &r.Pattern, (*string)(&r.State), &r.Iterations, &r.ToolCalls,
		&usageJSON, &r.Summary, &r.Input, &resultJSON, &r.Error, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, &NotFoundError{Kind: "run", ID: id}
		}
		return nil, fmt.Errorf("agentruntime: get run: %w", err)
	}
	if usageJSON.Valid {
		_ = json.Unmarshal([]byte(usageJSON.String), &r.Usage)
	}
	if resultJSON.Valid && resultJSON.String != "" {
		var res TaskResult
		if err := json.Unmarshal([]byte(resultJSON.String), &res); err == nil {
			r.Result = &res
		}
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt.String)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt.String)
	return &r, nil
}

func (s *SQLiteStorage) AddEvent(ctx context.Context, ev *RuntimeEvent) error {
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		return fmt.Errorf("agentruntime: marshal event data: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO events (id, run_id, kind, data_json, timestamp) VALUES (?,?,?,?,?)`,
		ev.ID, ev.TaskRunID, string(ev.Kind), string(dataJSON), ev.Timestamp.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("agentruntime: add event: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) Events(ctx context.Context, runID string) ([]RuntimeEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, kind, data_json, timestamp FROM events WHERE run_id = ? ORDER BY timestamp`, runID)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: query events: %w", err)
	}
	defer rows.Close()
	var out []RuntimeEvent
	for rows.Next() {
		var ev RuntimeEvent
		var dataJSON, ts string
		if err := rows.Scan(&ev.ID, &ev.TaskRunID, (*string)(&ev.Kind), &dataJSON, &ts); err != nil {
			return nil, fmt.Errorf("agentruntime: scan event: %w", err)
		}
		ev.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		ev.Data = map[string]any{}
		_ = json.Unmarshal([]byte(dataJSON), &ev.Data)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) AddArtifact(ctx context.Context, a *Artifact) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (id, run_id, name, type, content, created_at) VALUES (?,?,?,?,?,?)`,
		a.ID, a.TaskRunID, a.Name, string(a.Type), a.Content, a.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("agentruntime: add artifact: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) Artifacts(ctx context.Context, runID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, name, type, content, created_at FROM artifacts WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: query artifacts: %w", err)
	}
	defer rows.Close()
	var out []Artifact
	for rows.Next() {
		var a Artifact
		var ts string
		if err := rows.Scan(&a.ID, &a.TaskRunID, &a.Name, (*string)(&a.Type), &a.Content, &ts); err != nil {
			return nil, fmt.Errorf("agentruntime: scan artifact: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) AddEvidence(ctx context.Context, e *Evidence) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO evidence (artifact_id, source, quote) VALUES (?,?,?)`,
		e.ArtifactID, e.Source, e.Quote)
	if err != nil {
		return fmt.Errorf("agentruntime: add evidence: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) Evidence(ctx context.Context, artifactID string) ([]Evidence, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT artifact_id, source, quote FROM evidence WHERE artifact_id = ?`, artifactID)
	if err != nil {
		return nil, fmt.Errorf("agentruntime: query evidence: %w", err)
	}
	defer rows.Close()
	var out []Evidence
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(&e.ArtifactID, &e.Source, &e.Quote); err != nil {
			return nil, fmt.Errorf("agentruntime: scan evidence: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) Close() error { return s.db.Close() }

func nullable(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}
