module github.com/mantis-layer/mts/tools

go 1.25.4

require github.com/mantis-layer/mts/agent-core v0.0.0

require github.com/mantis-layer/mts/agent-model v0.0.0 // indirect

replace (
	github.com/mantis-layer/mts/agent-core => ../agent-core
	github.com/mantis-layer/mts/agent-model => ../agent-model
)
