package agentcontract

import (
	"fmt"
	"time"
)

// Persona 是 Agent 的持久化身份（FR-010）。
// 最小身份集：ID/Name/Role/SystemPrompt/CreatedAt/UpdatedAt；
// Preferences 与 Skills 不在此处，归属 Memory 的 preference/skill 层。
type Persona struct {
	ID           string
	Name         string
	Role         string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Validate 校验 Persona 的最小契约：ID 非空。
func (p Persona) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("agentcontract: Persona.ID 不能为空")
	}
	return nil
}
