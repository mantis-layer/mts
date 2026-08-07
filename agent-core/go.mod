module github.com/mantis-layer/mts/agent-core

go 1.25.4

require (
	github.com/mantis-layer/mts/agent-contract v0.0.0
	github.com/mantis-layer/mts/agent-model v0.0.0
)

replace (
	github.com/mantis-layer/mts/agent-contract => ../agent-contract
	github.com/mantis-layer/mts/agent-model => ../agent-model
)
