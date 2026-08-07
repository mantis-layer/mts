package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Options 配置 Agent 的行为。
type Options struct {
	// MaxIterations 模型调用轮次上限（默认 10），防止死循环。
	MaxIterations int
	// MaxToolCalls 单次 Run 工具调用预算（默认 20），超限停止（Issue A7）。
	MaxToolCalls int
	// ToolTimeout 单次工具执行超时（默认 30s），超时被取消（Issue A5）。
	ToolTimeout time.Duration
	// OnEvent 事件回调；为 nil 时静默运行。
	OnEvent func(Event)
	// Steering 每次模型调用前调用，可修改消息列表或中止运行。
	Steering func(ctx context.Context, msgs []agentmodel.Message) ([]agentmodel.Message, error)
	// ContextBuilder 在 Steering 之后、ContextHook 之前注入相关记忆（FR-012）；
	// 需同时配置 Persona 与 MemoryStore 才会被调用；为 nil 时跳过（与 v0.1 等价）。
	ContextBuilder agentcontract.ContextBuilder
	// Persona 是本次运行的 Agent 身份（FR-010）；为 nil 时跳过记忆注入。
	Persona *agentcontract.Persona
	// MemoryStore 是 Persona 的记忆存储（FR-011）；未配置时跳过记忆注入。
	MemoryStore agentcontract.MemoryStore
	// ContextHook 在 ContextBuilder 之后、模型调用之前变换输入消息（Context Transform Hook）。
	ContextHook func(ctx context.Context, msgs []agentmodel.Message) []agentmodel.Message
}

// Agent 是最小 Agent 循环运行时（FR-002）。
type Agent struct {
	model    agentmodel.Model
	registry *Registry
	opts     Options
}

// Result 是一次 Run 的结果。
type Result struct {
	FinalMessage agentmodel.Message
	Usage        agentmodel.Usage
	ToolCalls    int
	Iterations   int
	Aborted      bool
}

// New 构造 Agent。model 与 registry 不能为 nil。
func New(model agentmodel.Model, registry *Registry, opts Options) *Agent {
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 10
	}
	if opts.MaxToolCalls <= 0 {
		opts.MaxToolCalls = 20
	}
	if opts.ToolTimeout <= 0 {
		opts.ToolTimeout = 30 * time.Second
	}
	return &Agent{model: model, registry: registry, opts: opts}
}

// Run 以 input 作为用户消息执行一次 Agent 循环（FR-002 Model→Tool→Model）。
func (a *Agent) Run(ctx context.Context, input string) (Result, error) {
	return a.RunWithMessages(ctx, []agentmodel.Message{{Role: agentmodel.RoleUser, Content: input}})
}

// RunWithMessages 以已有消息上下文执行一次 Agent 循环。
func (a *Agent) RunWithMessages(ctx context.Context, msgs []agentmodel.Message) (Result, error) {
	res := Result{}
	a.emit(Event{Kind: EventRunStart})
	defer a.emit(Event{Kind: EventRunEnd})

	for {
		if err := ctx.Err(); err != nil {
			res.Aborted = true
			return res, err
		}
		if res.Iterations >= a.opts.MaxIterations {
			err := fmt.Errorf("agent: 达到最大迭代次数 %d，停止", a.opts.MaxIterations)
			a.emit(Event{Kind: EventAgentError, Error: err})
			return res, err
		}

		if a.opts.Steering != nil {
			var err error
			msgs, err = a.opts.Steering(ctx, msgs)
			if err != nil {
				a.emit(Event{Kind: EventAgentError, Error: err})
				return res, err
			}
		}
		view := a.applyContextBuilder(ctx, msgs)
		if a.opts.ContextHook != nil {
			view = a.opts.ContextHook(ctx, view)
		}

		a.emit(Event{Kind: EventModelStart, Model: a.modelName()})
		assistant, usage, err := a.callModel(ctx, view)
		if err != nil {
			if ctx.Err() != nil {
				res.Aborted = true
				return res, ctx.Err()
			}
			a.emit(Event{Kind: EventAgentError, Error: err})
			return res, err
		}
		res.Usage = mergeUsage(res.Usage, usage)
		res.Iterations++
		a.emit(Event{Kind: EventModelDone, Model: a.modelName(), Usage: &usage})

		if len(assistant.ToolCalls) == 0 {
			res.FinalMessage = assistant
			a.emit(Event{Kind: EventAgentMessage, Content: assistant.Content})
			return res, nil
		}

		msgs = append(msgs, assistant)
		for _, tc := range assistant.ToolCalls {
			if res.ToolCalls >= a.opts.MaxToolCalls {
				err := fmt.Errorf("agent: 工具调用预算耗尽（上限 %d），停止继续调用", a.opts.MaxToolCalls)
				a.emit(Event{Kind: EventAgentError, Error: err})
				return res, err
			}
			res.ToolCalls++
			a.emit(Event{Kind: EventToolStart, Tool: tc.Name, ToolCallID: tc.ID})
			resultMsg := a.executeToolAndEmit(ctx, tc)
			msgs = append(msgs, resultMsg)
		}
	}
}

// applyContextBuilder 在 Steering 之后、ContextHook 之前执行记忆注入（FR-012）。
// ContextBuilder/Persona/MemoryStore 任一未配置则跳过；
// Build 失败（如检索超时）降级为不注入，不阻塞主循环（Run 不 fail）。
func (a *Agent) applyContextBuilder(ctx context.Context, msgs []agentmodel.Message) []agentmodel.Message {
	if a.opts.ContextBuilder == nil || a.opts.Persona == nil || a.opts.MemoryStore == nil {
		return msgs
	}
	var current agentmodel.Message
	if len(msgs) > 0 {
		current = msgs[len(msgs)-1]
	}
	built, err := a.opts.ContextBuilder.Build(ctx, *a.opts.Persona, a.opts.MemoryStore, current)
	if err != nil {
		a.emit(Event{Kind: EventMemoryInjected, Error: err})
		return msgs
	}
	if built.Content == "" {
		return msgs
	}
	a.emit(Event{Kind: EventMemoryInjected, Content: built.Content})
	return append([]agentmodel.Message{built}, msgs...)
}

// callModel 调用模型并组装 assistant 消息（含流式收集）。
func (a *Agent) callModel(ctx context.Context, view []agentmodel.Message) (agentmodel.Message, agentmodel.Usage, error) {
	req := agentmodel.Request{Messages: view, Tools: a.registry.Schemas()}
	ch, err := a.model.Stream(ctx, req)
	if err != nil {
		return agentmodel.Message{}, agentmodel.Usage{}, err
	}

	assistant := agentmodel.Message{Role: agentmodel.RoleAssistant}
	var usage agentmodel.Usage
	var streamErr error
	for ev := range ch {
		switch ev.Kind {
		case agentmodel.StreamEventDelta:
			assistant.Content += ev.Delta
			a.emit(Event{Kind: EventModelDelta, Model: a.modelName(), Content: ev.Delta})
		case agentmodel.StreamEventToolCall:
			if ev.ToolCall != nil {
				assistant.ToolCalls = append(assistant.ToolCalls, *ev.ToolCall)
			}
		case agentmodel.StreamEventUsage:
			if ev.Usage != nil {
				usage = *ev.Usage
			}
		case agentmodel.StreamEventFinish:
			// finish reason 已隐含在是否有 ToolCalls 中，此处无需处理。
		case agentmodel.StreamEventError:
			if ev.Error != nil {
				streamErr = ev.Error
			}
		}
	}
	if streamErr != nil {
		return agentmodel.Message{}, agentmodel.Usage{}, streamErr
	}
	return assistant, usage, nil
}

// executeToolAndEmit 执行单个工具调用并产生对应事件，返回 tool 结果消息。
func (a *Agent) executeToolAndEmit(ctx context.Context, tc agentmodel.ToolCall) agentmodel.Message {
	resultMsg := agentmodel.Message{Role: agentmodel.RoleTool, ToolCallID: tc.ID, Name: tc.Name}

	output, err := a.executeTool(ctx, tc)
	if err != nil {
		a.emit(Event{Kind: EventToolError, Tool: tc.Name, ToolCallID: tc.ID, Error: err})
		// 结构化错误回给模型，让模型继续决策或给出解释。
		resultMsg.Content = fmt.Sprintf("{\"error\": %q}", err.Error())
		return resultMsg
	}

	data, marshalErr := json.Marshal(output)
	if marshalErr != nil {
		a.emit(Event{Kind: EventToolError, Tool: tc.Name, ToolCallID: tc.ID, Error: marshalErr})
		resultMsg.Content = fmt.Sprintf("{\"error\": %q}", marshalErr.Error())
		return resultMsg
	}
	resultMsg.Content = string(data)
	a.emit(Event{Kind: EventToolDone, Tool: tc.Name, ToolCallID: tc.ID, Content: string(data)})
	return resultMsg
}

// executeTool 校验并执行工具：查找 → 参数解析 → Schema 校验 → 带超时执行。
func (a *Agent) executeTool(ctx context.Context, tc agentmodel.ToolCall) (map[string]any, error) {
	tool, ok := a.registry.Get(tc.Name)
	if !ok {
		return nil, NewToolError("tool_not_found", "未注册的工具: "+tc.Name)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
		return nil, NewToolError("invalid_arguments", "工具参数不是合法 JSON: "+err.Error())
	}
	if err := ValidateJSONSchema(tool.Parameters(), input); err != nil {
		return nil, NewToolError("schema_validation", "工具输入未通过 Schema 校验: "+err.Error())
	}
	toolCtx, cancel := context.WithTimeout(ctx, a.opts.ToolTimeout)
	defer cancel()
	return tool.Execute(toolCtx, input)
}

func (a *Agent) modelName() string {
	if named, ok := a.model.(interface{ ModelName() string }); ok {
		return named.ModelName()
	}
	return ""
}

func (a *Agent) emit(ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if a.opts.OnEvent != nil {
		a.opts.OnEvent(ev)
	}
}

func mergeUsage(a, b agentmodel.Usage) agentmodel.Usage {
	return agentmodel.Usage{
		PromptTokens:     a.PromptTokens + b.PromptTokens,
		CompletionTokens: a.CompletionTokens + b.CompletionTokens,
		TotalTokens:      a.TotalTokens + b.TotalTokens,
	}
}
