module github.com/mantis-layer/mts/cli

go 1.25.4

require (
	github.com/mantis-layer/mts/adapters/model-openai v0.0.0
	github.com/mantis-layer/mts/agent-core v0.0.0
	github.com/mantis-layer/mts/tools v0.0.0
)

require github.com/mantis-layer/mts/agent-model v0.0.0 // indirect

replace (
	github.com/mantis-layer/mts/adapters/model-openai => ../adapters/model-openai
	github.com/mantis-layer/mts/agent-core => ../agent-core
	github.com/mantis-layer/mts/agent-model => ../agent-model
	github.com/mantis-layer/mts/tools => ../tools
)
