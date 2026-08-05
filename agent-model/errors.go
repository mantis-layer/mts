package agentmodel

// ErrorKind 标识模型错误的分类。
type ErrorKind string

const (
	// ErrorKindInvalidRequest 请求参数非法。
	ErrorKindInvalidRequest ErrorKind = "invalid_request"
	// ErrorKindAuthentication 认证失败（key 缺失/无效）。
	ErrorKindAuthentication ErrorKind = "authentication"
	// ErrorKindRateLimit 限流或配额不足。
	ErrorKindRateLimit ErrorKind = "rate_limit"
	// ErrorKindTimeout 请求超时。
	ErrorKindTimeout ErrorKind = "timeout"
	// ErrorKindNetwork 网络层错误。
	ErrorKindNetwork ErrorKind = "network"
	// ErrorKindServer 服务端错误。
	ErrorKindServer ErrorKind = "server"
	// ErrorKindUnknown 未分类错误。
	ErrorKindUnknown ErrorKind = "unknown"
)

// ModelError 是结构化的模型调用错误。
type ModelError struct {
	Kind      ErrorKind
	Message   string
	Retryable bool
	Cause     error
}

func (e *ModelError) Error() string {
	if e.Cause != nil {
		return "agentmodel: " + string(e.Kind) + ": " + e.Message + " (" + e.Cause.Error() + ")"
	}
	return "agentmodel: " + string(e.Kind) + ": " + e.Message
}

// Unwrap 支持 errors.Is / errors.As。
func (e *ModelError) Unwrap() error { return e.Cause }
