// Package tools 提供官方 Tool 实现：FileReader（本地 JSON/CSV 读取）
// 与 Calculator（安全数学表达式求值）。
package tools

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mantis-layer/mts/agent-core"
)

// FileReader 读取本地 JSON 或 CSV 文件并解析为结构化数据。
type FileReader struct{}

// Name 返回工具唯一 ID。
func (FileReader) Name() string { return "file_reader" }

// Description 描述工具用途。
func (FileReader) Description() string {
	return "读取本地 JSON 或 CSV 文件并解析为结构化数据，返回文件内容"
}

// Parameters 返回输入 JSON Schema。
func (FileReader) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "文件路径（JSON 或 CSV）"},
		},
		"required": []string{"path"},
	}
}

// Execute 读取并解析文件。
func (FileReader) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	path, _ := input["path"].(string)
	if path == "" {
		return nil, agentcore.NewToolError("invalid_argument", "path 必填")
	}
	if isForbiddenPath(path) {
		return nil, agentcore.NewToolError("forbidden_path", "禁止读取密钥或环境配置文件: "+path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, agentcore.NewToolError("file_not_found", "文件不存在: "+path)
		}
		return nil, agentcore.NewToolError("read_error", "读取失败: "+err.Error())
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, agentcore.NewToolError("parse_error", "JSON 解析失败: "+err.Error())
		}
		return map[string]any{"path": path, "data": v}, nil
	case ".csv":
		rows, err := parseCSV(data)
		if err != nil {
			return nil, agentcore.NewToolError("parse_error", "CSV 解析失败: "+err.Error())
		}
		return map[string]any{"path": path, "data": rows}, nil
	default:
		return nil, agentcore.NewToolError("unsupported_format", "不支持的文件格式: "+ext+"（仅支持 .json / .csv）")
	}
}

// isForbiddenPath 判断路径是否指向密钥/环境配置文件，防止经
// prompt injection 读取 .env.local 等敏感文件（NFR-004）。
// 覆盖：.env 前缀（大小写不敏感）、SSH 私钥（id_rsa/id_ed25519/id_ecdsa/id_dsa，
// 排除 .pub 公钥）、常见密钥扩展名。
func isForbiddenPath(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if strings.HasPrefix(lower, ".env") {
		return true
	}
	if strings.HasPrefix(lower, "id_") && !strings.HasSuffix(lower, ".pub") {
		return true
	}
	switch filepath.Ext(lower) {
	case ".pem", ".key", ".p12", ".pfx", ".ppk":
		return true
	}
	return false
}

// parseCSV 将首行作为表头，解析为 []map[string]string。
func parseCSV(data []byte) ([]map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []map[string]string{}, nil
	}
	header := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
