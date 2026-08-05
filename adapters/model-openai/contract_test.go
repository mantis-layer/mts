package modelopenai

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// skipIfNetworkUnreachable 在连接层不可达时跳过测试并记录原因。
// 环境允许列表可能只放行特定域名（如 GitHub API），导致中转站不可达；
// 这类环境限制不应误报为 API 失败。认证/服务端等真实响应仍会 FAIL。
func skipIfNetworkUnreachable(t *testing.T, err error) bool {
	t.Helper()
	var me *agentmodel.ModelError
	if errors.As(err, &me) && (me.Kind == agentmodel.ErrorKindNetwork || me.Kind == agentmodel.ErrorKindTimeout) {
		t.Skipf("端点网络不可达（环境限制，非 API 失败）: %v", err)
		return true
	}
	return false
}

// testEndpoint 从环境读取一个 OpenAI 兼容端点配置。
type testEndpoint struct {
	name    string
	baseURL string
	apiKey  string
	model   string
}

// loadEndpoints 读取 MTS_* 与 MTS_*2 两组端点；返回 ok=false 表示无任何端点可用。
func loadEndpoints(t *testing.T) (eps []testEndpoint, ok bool) {
	t.Helper()
	add := func(prefix string) {
		baseURL := os.Getenv(prefix + "_BASEURL")
		apiKey := os.Getenv(prefix + "_API_KEY")
		model := os.Getenv(prefix + "_MODEL")
		if baseURL == "" || apiKey == "" || model == "" {
			return
		}
		eps = append(eps, testEndpoint{name: prefix, baseURL: baseURL, apiKey: apiKey, model: model})
	}
	add("MTS")
	add("MTS2")
	if len(eps) == 0 {
		t.Log("未配置 MTS_BASEURL/MTS_API_KEY/MTS_MODEL，契约测试跳过")
		return nil, false
	}
	return eps, true
}

func newTestClient(t *testing.T, ep testEndpoint) *Client {
	t.Helper()
	// 短超时：环境不可达时快速判定为 SKIP，而非长时间挂起。
	hc := &http.Client{Timeout: 8 * time.Second}
	c, err := New(Config{BaseURL: ep.baseURL, APIKey: ep.apiKey, Model: ep.model, HTTPClient: hc})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	return c
}

func userMsg(content string) []agentmodel.Message {
	return []agentmodel.Message{{Role: agentmodel.RoleUser, Content: content}}
}

// TestContract_NonStreamingText 验证非流式文本补全与 Usage。
func TestContract_NonStreamingText(t *testing.T) {
	eps, ok := loadEndpoints(t)
	if !ok {
		t.Skip("无测试端点")
	}
	for _, ep := range eps {
		t.Run(ep.name, func(t *testing.T) {
			c := newTestClient(t, ep)
			resp, err := c.Complete(context.Background(), agentmodel.Request{
				Messages: userMsg("请只回复两个字：你好"),
			})
			if err != nil {
				if skipIfNetworkUnreachable(t, err) {
					return
				}
				t.Fatalf("Complete 失败: %v", err)
			}
			if strings.TrimSpace(resp.Message.Content) == "" {
				t.Fatal("回复内容为空")
			}
			if resp.Usage.TotalTokens <= 0 {
				t.Fatalf("Usage 未返回: %+v", resp.Usage)
			}
			t.Logf("reply=%q usage=%+v finish=%s", resp.Message.Content, resp.Usage, resp.FinishReason)
		})
	}
}

// TestContract_StreamingText 验证流式文本补全：delta 增量 + finish 事件。
func TestContract_StreamingText(t *testing.T) {
	eps, ok := loadEndpoints(t)
	if !ok {
		t.Skip("无测试端点")
	}
	for _, ep := range eps {
		t.Run(ep.name, func(t *testing.T) {
			c := newTestClient(t, ep)
			ch, err := c.Stream(context.Background(), agentmodel.Request{
				Messages: userMsg("请只回复两个字：你好"),
			})
			if err != nil {
				if skipIfNetworkUnreachable(t, err) {
					return
				}
				t.Fatalf("Stream 启动失败: %v", err)
			}
			var got strings.Builder
			sawFinish := false
			for ev := range ch {
				switch ev.Kind {
				case agentmodel.StreamEventDelta:
					got.WriteString(ev.Delta)
				case agentmodel.StreamEventFinish:
					sawFinish = true
				case agentmodel.StreamEventError:
					if ev.Error != nil {
						t.Fatalf("流中错误: %v", ev.Error)
					}
				}
			}
			if !sawFinish {
				t.Fatal("未收到 finish 事件")
			}
			if strings.TrimSpace(got.String()) == "" {
				t.Fatal("流式内容为空")
			}
			t.Logf("streamed=%q", got.String())
		})
	}
}

// TestContract_ToolCall 验证模型能发起工具调用并返回结构化 tool_call。
func TestContract_ToolCall(t *testing.T) {
	eps, ok := loadEndpoints(t)
	if !ok {
		t.Skip("无测试端点")
	}
	tool := agentmodel.NewToolSchema("calculator", "计算数学表达式", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "数学表达式，如 1+2"},
		},
		"required": []string{"expression"},
	})
	for _, ep := range eps {
		t.Run(ep.name, func(t *testing.T) {
			c := newTestClient(t, ep)
			resp, err := c.Complete(context.Background(), agentmodel.Request{
				Messages: userMsg("请用 calculator 工具计算 1+2，只调用工具，不要输出其他内容"),
				Tools:    []agentmodel.ToolSchema{tool},
			})
			if err != nil {
				if skipIfNetworkUnreachable(t, err) {
					return
				}
				t.Fatalf("Complete 失败: %v", err)
			}
			if len(resp.Message.ToolCalls) == 0 {
				t.Fatalf("未产生 tool_call（finish=%s content=%q）", resp.FinishReason, resp.Message.Content)
			}
			tc := resp.Message.ToolCalls[0]
			if tc.Name != "calculator" {
				t.Fatalf("工具名=%q 期望 calculator", tc.Name)
			}
			if tc.Arguments == "" {
				t.Fatal("tool_call 参数为空")
			}
			t.Logf("tool_call=%+v", tc)
		})
	}
}

// TestContract_AuthenticationError 验证无效 key 映射为 authentication 错误。
func TestContract_AuthenticationError(t *testing.T) {
	eps, ok := loadEndpoints(t)
	if !ok {
		t.Skip("无测试端点")
	}
	c, err := New(Config{BaseURL: eps[0].baseURL, APIKey: "sk-invalid-key-for-test", Model: eps[0].model})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Complete(context.Background(), agentmodel.Request{Messages: userMsg("hi")})
	if err == nil {
		t.Fatal("期望认证错误，实际无错误")
	}
	if skipIfNetworkUnreachable(t, err) {
		return
	}
	var me *agentmodel.ModelError
	if !asModelError(err, &me) {
		t.Fatalf("期望 ModelError，实际 %T: %v", err, err)
	}
	if me.Kind != agentmodel.ErrorKindAuthentication {
		t.Fatalf("错误分类=%s 期望 authentication（错误: %v）", me.Kind, err)
	}
	t.Logf("认证错误映射正确: %v", me)
}

// TestRedactSecrets 验证错误体中的 Bearer token 与 sk- 密钥被脱敏。
func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"error":"unauthorized","auth":"Bearer sk-acw-92b51d2e-9bec066031ebac40"}`, `{"error":"unauthorized","auth":"Bearer [REDACTED]"}`},
		{`token sk-proj-abcdefgh12345678 leaked`, `token sk-[REDACTED] leaked`},
		{`Authorization: bearer abc123XYZ_-+/=`, `Authorization: Bearer [REDACTED]`},
		{`clean message`, `clean message`},
	}
	for _, cse := range cases {
		got := redactSecrets(cse.in)
		if got != cse.want {
			t.Fatalf("redactSecrets(%q) = %q, 期望 %q", cse.in, got, cse.want)
		}
	}
}

func asModelError(err error, target **agentmodel.ModelError) bool {
	me, ok := err.(*agentmodel.ModelError)
	if ok {
		*target = me
	}
	return ok
}
