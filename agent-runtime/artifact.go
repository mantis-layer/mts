package agentruntime

import "time"

// ArtifactType 标识 Artifact 的类型。
type ArtifactType string

const (
	ArtifactText  ArtifactType = "text"
	ArtifactJSON  ArtifactType = "json"
	ArtifactTable ArtifactType = "table"
)

// Artifact 是 TaskRun 的结构化产出（FR-004）。
type Artifact struct {
	ID        string
	TaskRunID string
	Name      string
	Type      ArtifactType
	Content   string // JSON 编码内容
	CreatedAt time.Time
}

// Evidence 将 Artifact 内容关联到来源（供 Evaluator 与引用）。
type Evidence struct {
	ArtifactID string
	Source     string
	Quote      string
}
