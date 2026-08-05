package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mantis-layer/mts/agent-core"
)

// Calculator 计算数学表达式（支持 + - * / %、括号、小数与一元负号）。
type Calculator struct{}

// Name 返回工具唯一 ID。
func (Calculator) Name() string { return "calculator" }

// Description 描述工具用途。
func (Calculator) Description() string {
	return "计算数学表达式并返回数值结果，支持四则运算、括号、小数（如 (1+2)*3.5）"
}

// Parameters 返回输入 JSON Schema。
func (Calculator) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "数学表达式，如 (1+2)*3"},
		},
		"required": []string{"expression"},
	}
}

// Execute 求值表达式。
func (Calculator) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	expr, _ := input["expression"].(string)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, agentcore.NewToolError("invalid_argument", "expression 必填")
	}
	result, err := evalExpression(expr)
	if err != nil {
		return nil, agentcore.NewToolError("invalid_expression", "表达式求值失败: "+err.Error())
	}
	return map[string]any{"expression": expr, "result": result}, nil
}

// ---- 安全表达式求值器（shunting-yard + RPN），无 eval，无外部依赖 ----

type tokenKind int

const (
	tokNumber tokenKind = iota
	tokOp
	tokLParen
	tokRParen
)

type token struct {
	kind tokenKind
	num  float64
	op   string
}

func tokenize(expr string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(expr) {
		c := expr[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c >= '0' && c <= '9' || c == '.':
			j := i
			for j < len(expr) && (expr[j] >= '0' && expr[j] <= '9' || expr[j] == '.') {
				j++
			}
			num, err := strconv.ParseFloat(expr[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("非法数字 %q", expr[i:j])
			}
			toks = append(toks, token{kind: tokNumber, num: num})
			i = j
		case c == '+' || c == '-' || c == '*' || c == '/' || c == '%':
			toks = append(toks, token{kind: tokOp, op: string(c)})
			i++
		case c == '(':
			toks = append(toks, token{kind: tokLParen})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokRParen})
			i++
		default:
			return nil, fmt.Errorf("非法字符 %q", string(c))
		}
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("空表达式")
	}
	return toks, nil
}

// precedence 返回运算符优先级（* / % 高于 + -）。
func precedence(op string) int {
	switch op {
	case "+", "-":
		return 1
	case "*", "/", "%":
		return 2
	}
	return 0
}

// toRPN 使用 shunting-yard 将中缀 token 转为后缀（RPN）。
// 支持一元负号：数字前或 '(' 后的 '-' 视为一元。
func toRPN(toks []token) ([]token, error) {
	var out []token
	var stack []token
	prevNum := false // 上一个 token 是否终结一个操作数
	for i, t := range toks {
		switch t.kind {
		case tokNumber:
			out = append(out, t)
			prevNum = true
		case tokLParen:
			stack = append(stack, t)
			prevNum = false
		case tokRParen:
			for len(stack) > 0 && stack[len(stack)-1].kind != tokLParen {
				out = append(out, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return nil, fmt.Errorf("括号不匹配")
			}
			stack = stack[:len(stack)-1] // 弹出 '('
			prevNum = true
		case tokOp:
			// 一元负号/正号：表达式开头、'(' 后或二元运算符后
			unary := !prevNum && (t.op == "-" || t.op == "+")
			if unary {
				out = append(out, token{kind: tokNumber, num: 0})
			}
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				if top.kind != tokOp {
					break
				}
				if precedence(top.op) >= precedence(t.op) {
					out = append(out, top)
					stack = stack[:len(stack)-1]
				} else {
					break
				}
			}
			stack = append(stack, t)
			prevNum = false
		}
		_ = i
	}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		if top.kind == tokLParen {
			return nil, fmt.Errorf("括号不匹配")
		}
		out = append(out, top)
		stack = stack[:len(stack)-1]
	}
	return out, nil
}

// evalRPN 求值后缀表达式。
func evalRPN(rpn []token) (float64, error) {
	var stack []float64
	for _, t := range rpn {
		if t.kind == tokNumber {
			stack = append(stack, t.num)
			continue
		}
		if len(stack) < 2 {
			return 0, fmt.Errorf("表达式不完整")
		}
		b := stack[len(stack)-1]
		a := stack[len(stack)-2]
		stack = stack[:len(stack)-2]
		var r float64
		switch t.op {
		case "+":
			r = a + b
		case "-":
			r = a - b
		case "*":
			r = a * b
		case "/":
			if b == 0 {
				return 0, fmt.Errorf("除数为零")
			}
			r = a / b
		case "%":
			if b == 0 {
				return 0, fmt.Errorf("模数为零")
			}
			r = float64(int64(a) % int64(b))
		}
		stack = append(stack, r)
	}
	if len(stack) != 1 {
		return 0, fmt.Errorf("表达式不完整")
	}
	return stack[0], nil
}

// evalExpression 对表达式求值。
func evalExpression(expr string) (float64, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return 0, err
	}
	rpn, err := toRPN(toks)
	if err != nil {
		return 0, err
	}
	return evalRPN(rpn)
}
