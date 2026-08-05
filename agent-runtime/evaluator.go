package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
)

// EvaluationResult 是 Evaluator 的一次评估结果。
type EvaluationResult struct {
	Passed  bool           `json:"passed"`
	Score   float64        `json:"score"`
	Details map[string]any `json:"details,omitempty"`
}

// Evaluator 验收 TaskRun 的产出（FR-004）。
type Evaluator interface {
	Name() string
	Evaluate(ctx context.Context, run *TaskRun, store Storage) (*EvaluationResult, error)
}

// SchemaEvaluator 验证指定 Artifact 的 Content 是合法 JSON（基础 Evaluator）。
type SchemaEvaluator struct {
	ArtifactName string
}

// Name 返回 Evaluator 名。
func (e *SchemaEvaluator) Name() string { return "schema" }

// Evaluate 检查 Artifact 存在且 Content 可解析为 JSON。
func (e *SchemaEvaluator) Evaluate(ctx context.Context, run *TaskRun, store Storage) (*EvaluationResult, error) {
	arts, err := store.Artifacts(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	for _, a := range arts {
		if a.Name == e.ArtifactName {
			var v any
			if err := json.Unmarshal([]byte(a.Content), &v); err != nil {
				return &EvaluationResult{Passed: false, Score: 0, Details: map[string]any{"error": err.Error()}}, nil
			}
			return &EvaluationResult{Passed: true, Score: 1, Details: map[string]any{"artifact": a.Name}}, nil
		}
	}
	return &EvaluationResult{Passed: false, Score: 0, Details: map[string]any{"error": fmt.Sprintf("artifact %q 不存在", e.ArtifactName)}}, nil
}

// EvidenceCoverageEvaluator 计算 Evidence 覆盖率（基础 Evaluator）。
type EvidenceCoverageEvaluator struct {
	Required int
}

// Name 返回 Evaluator 名。
func (e *EvidenceCoverageEvaluator) Name() string { return "evidence_coverage" }

// Evaluate 统计 run 内全部 Artifact 关联的 Evidence 条数并计算覆盖率。
func (e *EvidenceCoverageEvaluator) Evaluate(ctx context.Context, run *TaskRun, store Storage) (*EvaluationResult, error) {
	arts, err := store.Artifacts(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	evCount := 0
	for _, a := range arts {
		evs, err := store.Evidence(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		evCount += len(evs)
	}
	if e.Required <= 0 {
		return &EvaluationResult{Passed: evCount > 0, Score: 0, Details: map[string]any{"evidence": evCount}}, nil
	}
	score := float64(evCount) / float64(e.Required)
	if score > 1 {
		score = 1
	}
	return &EvaluationResult{Passed: evCount >= e.Required, Score: score, Details: map[string]any{"evidence": evCount, "required": e.Required}}, nil
}
