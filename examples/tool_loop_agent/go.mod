module github.com/mantis-layer/mts/examples/tool_loop_agent

go 1.25.4

require (
	github.com/mantis-layer/mts/adapters/model-openai v0.0.0
	github.com/mantis-layer/mts/agent-compose v0.0.0
	github.com/mantis-layer/mts/agent-core v0.0.0
	github.com/mantis-layer/mts/tools v0.0.0
)

require (
	github.com/mantis-layer/mts/agent-model v0.0.0 // indirect
	github.com/mantis-layer/mts/agent-plugin v0.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/mantis-layer/mts/adapters/model-openai => ../../adapters/model-openai
	github.com/mantis-layer/mts/agent-compose => ../../agent-compose
	github.com/mantis-layer/mts/agent-core => ../../agent-core
	github.com/mantis-layer/mts/agent-model => ../../agent-model
	github.com/mantis-layer/mts/agent-plugin => ../../agent-plugin
	github.com/mantis-layer/mts/tools => ../../tools
)
