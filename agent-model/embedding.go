package agentmodel

import "context"

// EmbeddingProvider 抽象文本向量嵌入生成能力，与 Model 并列、职责分离。
// 具体 Provider（OpenAI 兼容端点等）实现该接口。
type EmbeddingProvider interface {
	// Embed 将 inputs 批量转换为向量嵌入，返回与 inputs 一一对应的向量。
	// 空输入返回空结果且不报错；结果的向量维度必须全部一致且非零。
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}
