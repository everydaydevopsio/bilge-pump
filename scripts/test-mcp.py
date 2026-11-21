#!/usr/bin/env python3
"""Exercise the built MCP server over its real stdio transport."""

import json
import select
import subprocess
import sys
import tempfile


def request(process, request_id, method, params=None):
    message = {"jsonrpc": "2.0", "id": request_id, "method": method}
    if params is not None:
        message["params"] = params
    process.stdin.write(json.dumps(message) + "\n")
    process.stdin.flush()
    assert select.select([process.stdout], [], [], 30)[0], f"MCP {method} timed out"
    response = json.loads(process.stdout.readline())
    assert response["id"] == request_id, response
    assert "error" not in response, response
    return response["result"]


def main():
    with tempfile.TemporaryDirectory() as scan_path:
        process = subprocess.Popen(
            [sys.argv[1], "mcp"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            initialized = request(
                process,
                1,
                "initialize",
                {"protocolVersion": "2025-06-18", "capabilities": {},
                 "clientInfo": {"name": "smoke-test", "version": "1"}},
            )
            assert initialized["serverInfo"]["name"] == "bilge", initialized
            names = {tool["name"] for tool in request(process, 2, "tools/list")["tools"]}
            assert {"bilge_scan", "bilge_clean", "bilge_list_targets"} <= names, names
            targets = request(process, 3, "tools/call", {
                "name": "bilge_list_targets", "arguments": {},
            })
            assert "npm" in targets["content"][0]["text"], targets
            scan = request(process, 4, "tools/call", {
                "name": "bilge_scan",
                "arguments": {"task": "dev-cache", "scan_path": scan_path},
            })
            report = json.loads(scan["content"][0]["text"])
            assert report["dry_run"] is True, report
            assert report["tasks"][0]["task"] == "dev-cache", report
            rejected = request(process, 5, "tools/call", {
                "name": "bilge_clean", "arguments": {"task": "dev-cache"},
            })
            assert rejected["isError"] is True, rejected
        finally:
            process.stdin.close()
            process.wait(timeout=10)
            assert process.returncode == 0, process.stderr.read()


if __name__ == "__main__":
    main()
