package agentruntime

import contract "github.com/mantis-layer/mts/agent-contract"

// ArtifactType 上移至 agent-contract；runtime 通过 type alias 保持 API 兼容。
type ArtifactType = contract.ArtifactType

// Artifact 上移至 agent-contract。
type Artifact = contract.Artifact

// Evidence 上移至 agent-contract。
type Evidence = contract.Evidence

// Re-export constants。
const (
	ArtifactText  = contract.ArtifactText
	ArtifactJSON  = contract.ArtifactJSON
	ArtifactTable = contract.ArtifactTable
)
