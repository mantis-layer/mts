package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantis-layer/mts/agent-core"
)

func TestCalculator_Eval(t *testing.T) {
	c := Calculator{}
	cases := []struct {
		expr string
		want float64
	}{
		{"1+2", 3},
		{"(1+2)*3", 9},
		{"10/4", 2.5},
		{"-5+10", 5},
		{"2*(3+4)-1", 13},
		{"7%3", 1},
	}
	for _, cse := range cases {
		out, err := c.Execute(context.Background(), map[string]any{"expression": cse.expr})
		if err != nil {
			t.Fatalf("%q 求值失败: %v", cse.expr, err)
		}
		got, _ := out["result"].(float64)
		if got != cse.want {
			t.Fatalf("%q = %v, 期望 %v", cse.expr, got, cse.want)
		}
	}
}

func TestCalculator_Invalid(t *testing.T) {
	c := Calculator{}
	// 空表达式是参数错误
	_, err := c.Execute(context.Background(), map[string]any{"expression": ""})
	var te *agentcore.ToolError
	if !asToolError(err, &te) || te.Code != "invalid_argument" {
		t.Fatalf("空表达式错误=%v, 期望 invalid_argument", err)
	}
	// 其余为表达式求值错误
	bad := []string{"1+", "()", "a+b", "1/0", "1 2 3", "*5"}
	for _, expr := range bad {
		_, err := c.Execute(context.Background(), map[string]any{"expression": expr})
		if err == nil {
			t.Fatalf("%q 应报错", expr)
		}
		var te *agentcore.ToolError
		if ok := asToolError(err, &te); !ok || te.Code != "invalid_expression" {
			t.Fatalf("%q 错误=%v, 期望 invalid_expression", expr, err)
		}
	}
}

func TestFileReader_NotFound(t *testing.T) {
	f := FileReader{}
	_, err := f.Execute(context.Background(), map[string]any{"path": filepath.Join(t.TempDir(), "nope.json")})
	var te *agentcore.ToolError
	if !asToolError(err, &te) || te.Code != "file_not_found" {
		t.Fatalf("期望 file_not_found, 实际 %v", err)
	}
}

func TestFileReader_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"sales": [100, 200, 300]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := FileReader{}
	out, err := f.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatalf("data=%T", out["data"])
	}
	if _, ok := data["sales"]; !ok {
		t.Fatalf("缺 sales 字段: %v", data)
	}
}

func TestFileReader_CSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	if err := os.WriteFile(path, []byte("name,value\na,1\nb,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := FileReader{}
	out, err := f.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	rows, ok := out["data"].([]map[string]string)
	if !ok {
		t.Fatalf("data=%T", out["data"])
	}
	if len(rows) != 2 || rows[0]["name"] != "a" || rows[0]["value"] != "1" {
		t.Fatalf("CSV 解析错误: %v", rows)
	}
}

func TestFileReader_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := FileReader{}
	_, err := f.Execute(context.Background(), map[string]any{"path": path})
	var te *agentcore.ToolError
	if !asToolError(err, &te) || te.Code != "unsupported_format" {
		t.Fatalf("期望 unsupported_format, 实际 %v", err)
	}
}

func TestFileReader_ForbiddenPath(t *testing.T) {
	dir := t.TempDir()
	// .env 类、SSH 私钥与密钥类文件禁止读取（NFR-004 防 prompt injection 泄露）
	for _, name := range []string{".env.local", ".env", ".ENV", "server.key", "cert.pem", "id_rsa", "id_ed25519", "id_ecdsa", "ssh-key.ppk"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		f := FileReader{}
		_, err := f.Execute(context.Background(), map[string]any{"path": path})
		var te *agentcore.ToolError
		if !asToolError(err, &te) || te.Code != "forbidden_path" {
			t.Fatalf("%s: 期望 forbidden_path, 实际 %v", name, err)
		}
	}
	// 公钥文件不因敏感拦截（允许路径层面检查通过；格式不支持则是另一回事）
	pub := filepath.Join(dir, "id_rsa.pub")
	if err := os.WriteFile(pub, []byte("ssh-rsa AAAA..."), 0o600); err != nil {
		t.Fatal(err)
	}
	f := FileReader{}
	_, err := f.Execute(context.Background(), map[string]any{"path": pub})
	var te *agentcore.ToolError
	if asToolError(err, &te) && te.Code == "forbidden_path" {
		t.Fatalf("公钥不应被敏感拦截: %v", err)
	}
}

func asToolError(err error, target **agentcore.ToolError) bool {
	te, ok := err.(*agentcore.ToolError)
	if ok {
		*target = te
	}
	return ok
}
