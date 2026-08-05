module github.com/mantis-layer/mts/agent-plugin

go 1.25.4

require (
	github.com/mantis-layer/mts/agent-core v0.0.0
	github.com/mantis-layer/mts/agent-model v0.0.0
)

replace (
	github.com/mantis-layer/mts/agent-core => ../agent-core
	github.com/mantis-layer/mts/agent-model => ../agent-model
)
