#!/usr/bin/env python3
"""最小 MCP stdio server（示例，零依赖）。

实现 MCP 核心子集：
- initialize / notifications/initialized
- tools/list / tools/call

协议：JSON-RPC 2.0 over stdio，每行一条 JSON（与 Go 端 agent-plugin/mcp 兼容）。

工具：
- echo(msg)         —— 回显消息
- sum(numbers[])    —— 数字求和

运行：python3 python_echo_server.py
（由 Go 端 mcp.NewClient("python3", "python_echo_server.py") 启动）
"""

import json
import sys


TOOLS = [
    {
        "name": "echo",
        "description": "回显传入的文本消息",
        "inputSchema": {
            "type": "object",
            "properties": {"msg": {"type": "string"}},
            "required": ["msg"],
        },
    },
    {
        "name": "sum",
        "description": "计算一组数字的总和",
        "inputSchema": {
            "type": "object",
            "properties": {"numbers": {"type": "array", "items": {"type": "number"}}},
            "required": ["numbers"],
        },
    },
]


def handle_request(msg):
    method = msg.get("method")
    params = msg.get("params") or {}
    if not isinstance(params, dict):  # 防御：非对象 params
        params = {}
    if method == "initialize":
        return {
            "protocolVersion": "2024-11-05",
            "capabilities": {"tools": {}},
            "serverInfo": {"name": "python-echo-server", "version": "0.1.0"},
        }
    if method == "tools/list":
        return {"tools": TOOLS}
    if method == "tools/call":
        try:
            name = params.get("name")
            args = params.get("arguments") or {}
            if not isinstance(args, dict):
                raise ValueError("arguments 必须是对象")
            if name == "echo":
                text = str(args.get("msg", ""))
                return {"content": [{"type": "text", "text": f"echo: {text}"}]}
            if name == "sum":
                numbers = args.get("numbers", [])
                total = sum(float(n) for n in numbers)
                text = str(int(total)) if float(total).is_integer() else str(total)
                return {"content": [{"type": "text", "text": text}]}
            return {
                "content": [{"type": "text", "text": f"未知工具: {name}"}],
                "isError": True,
            }
        except (TypeError, ValueError, OverflowError, AttributeError) as exc:
            # 非法参数不得使 server 进程崩溃：返回 isError（协议级容错）。
            return {
                "content": [{"type": "text", "text": f"参数错误: {exc}"}],
                "isError": True,
            }
    # notifications 等无响应方法
    return None


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except (json.JSONDecodeError, RecursionError):  # 超深嵌套 JSON 不崩溃
            continue
        if "id" not in msg:  # notification
            continue
        try:
            result = handle_request(msg)
        except Exception as exc:  # 兜底：任何未预期异常 → 标准内部错误
            sys.stdout.write(json.dumps({
                "jsonrpc": "2.0",
                "id": msg["id"],
                "error": {"code": -32603, "message": f"internal error: {exc}"},
            }) + "\n")
            sys.stdout.flush()
            continue
        if result is None:  # 未知方法：返回 JSON-RPC method not found
            sys.stdout.write(json.dumps({
                "jsonrpc": "2.0",
                "id": msg["id"],
                "error": {"code": -32601, "message": "method not found"},
            }) + "\n")
            sys.stdout.flush()
            continue
        sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": msg["id"], "result": result}) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
