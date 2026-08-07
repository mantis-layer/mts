module github.com/mantis-layer/mts/examples/integration

go 1.25.4

require (
	github.com/mantis-layer/mts/agent-contract v0.0.0
	github.com/mantis-layer/mts/agent-core v0.0.0
	github.com/mantis-layer/mts/agent-model v0.0.0
	github.com/mantis-layer/mts/agent-runtime v0.0.0
	modernc.org/sqlite v1.56.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mantis-layer/mts/agent-plugin v0.0.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace (
	github.com/mantis-layer/mts/agent-contract => ../../agent-contract
	github.com/mantis-layer/mts/agent-core => ../../agent-core
	github.com/mantis-layer/mts/agent-model => ../../agent-model
	github.com/mantis-layer/mts/agent-plugin => ../../agent-plugin
	github.com/mantis-layer/mts/agent-runtime => ../../agent-runtime
)
