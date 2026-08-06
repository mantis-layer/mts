package modelopenai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// embedServer 构造一个返回固定 embedding 响应的 mock 端点。
func embedServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("请求路径=%q 期望 /embeddings", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
}

func embeddingResponse(data []struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}) string {
	b, _ := json.Marshal(map[string]any{"data": data})
	return string(b)
}

// TestEmbed_Success 验证正常路径：返回顺序与输入一致、维度一致、请求体正确。
func TestEmbed_Success(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{
		{Index: 1, Embedding: []float32{0.5, 0.25}},
		{Index: 0, Embedding: []float32{0.1, 0.9}},
	}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "chat-model", EmbeddingModel: "embed-model"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	vecs, err := c.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("向量数量=%d 期望 2", len(vecs))
	}
	// 乱序 index 应按 index 排序后返回
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.5 {
		t.Fatalf("向量顺序错误: %v", vecs)
	}
}

// TestEmbed_EmptyInput 验证空输入语义：返回空结果且不报错。
func TestEmbed_EmptyInput(t *testing.T) {
	c, err := New(Config{BaseURL: "http://unused.invalid", APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	vecs, err := c.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if vecs == nil || len(vecs) != 0 {
		t.Fatalf("空输入应返回空切片, 实际 %v", vecs)
	}
}

// TestEmbed_EmbeddingModelFallback 验证 EmbeddingModel 为空时回退到 Model。
func TestEmbed_EmbeddingModelFallback(t *testing.T) {
	got := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		got = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(embeddingResponse([]struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}{{Index: 0, Embedding: []float32{0.1}}})))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "chat-model"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	if _, err := c.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	if got != "chat-model" {
		t.Fatalf("回退模型=%q 期望 chat-model", got)
	}
}

// TestEmbed_DimensionMismatch 验证维度不一致报结构化错误。
func TestEmbed_DimensionMismatch(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{
		{Index: 0, Embedding: []float32{0.1, 0.2}},
		{Index: 1, Embedding: []float32{0.3}},
	}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("期望维度不一致错误，实际无错误")
	}
	assertModelErrorKind(t, err, agentmodel.ErrorKindServer)
}

// TestEmbed_ZeroDimension 验证零维向量报错。
func TestEmbed_ZeroDimension(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{{Index: 0, Embedding: []float32{}}}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("期望零维错误，实际无错误")
	}
	assertModelErrorKind(t, err, agentmodel.ErrorKindServer)
}

// TestEmbed_CountMismatch 验证返回条数不匹配报错。
func TestEmbed_CountMismatch(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{{Index: 0, Embedding: []float32{0.1}}}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("期望数量不匹配错误，实际无错误")
	}
	assertModelErrorKind(t, err, agentmodel.ErrorKindServer)
}

// TestEmbed_InvalidIndex 验证 index 乱序不连续报错。
func TestEmbed_InvalidIndex(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{
		{Index: 0, Embedding: []float32{0.1}},
		{Index: 2, Embedding: []float32{0.2}},
	}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("期望 index 错误，实际无错误")
	}
	assertModelErrorKind(t, err, agentmodel.ErrorKindServer)
}

// TestEmbed_MissingIndex 验证端点不返回 index（全 0）时信任返回顺序。
func TestEmbed_MissingIndex(t *testing.T) {
	srv := embedServer(t, embeddingResponse([]struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}{
		{Index: 0, Embedding: []float32{0.1}},
		{Index: 0, Embedding: []float32{0.2}},
	}), http.StatusOK)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	vecs, err := c.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed 失败: %v", err)
	}
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.2 {
		t.Fatalf("向量顺序错误: %v", vecs)
	}
}

// TestEmbed_RateLimit 验证 429 映射为 Retryable 的 rate_limit 错误。
func TestEmbed_RateLimit(t *testing.T) {
	srv := embedServer(t, `{"error":{"message":"rate limited"}}`, http.StatusTooManyRequests)
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("期望限流错误，实际无错误")
	}
	var me *agentmodel.ModelError
	if !errors.As(err, &me) {
		t.Fatalf("期望 ModelError，实际 %T: %v", err, err)
	}
	if me.Kind != agentmodel.ErrorKindRateLimit || !me.Retryable {
		t.Fatalf("错误=%+v 期望 rate_limit 且 Retryable", me)
	}
}

// TestEmbed_NetworkError 验证连接失败映射为结构化 network 错误。
func TestEmbed_NetworkError(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: "test-key", Model: "m"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	_, err = c.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("期望网络错误，实际无错误")
	}
	var me *agentmodel.ModelError
	if !errors.As(err, &me) {
		t.Fatalf("期望 ModelError，实际 %T: %v", err, err)
	}
	if me.Kind != agentmodel.ErrorKindNetwork {
		t.Fatalf("错误分类=%s 期望 network", me.Kind)
	}
}

func assertModelErrorKind(t *testing.T, err error, kind agentmodel.ErrorKind) {
	t.Helper()
	var me *agentmodel.ModelError
	if !errors.As(err, &me) {
		t.Fatalf("期望 ModelError，实际 %T: %v", err, err)
	}
	if me.Kind != kind {
		t.Fatalf("错误分类=%s 期望 %s（错误: %v）", me.Kind, kind, err)
	}
}
