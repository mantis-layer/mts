module github.com/mantis-layer/mts/agent-compose

go 1.25.4

require (
	github.com/mantis-layer/mts/agent-core v0.0.0
	github.com/mantis-layer/mts/agent-model v0.0.0
	github.com/mantis-layer/mts/agent-plugin v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	github.com/mantis-layer/mts/agent-core => ../agent-core
	github.com/mantis-layer/mts/agent-model => ../agent-model
	github.com/mantis-layer/mts/agent-plugin => ../agent-plugin
)
