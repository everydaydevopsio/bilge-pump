---
name: bilge-disk-savings
description: Find reclaimable developer disk space with Bilge when a user says storage is low, disk space is tight, or asks what caches can be cleaned. Use the Bilge MCP server for scans and targeted cleanup. Do not use for unrelated filesystem administration.
---

# Find disk savings with Bilge

Use the `bilge` MCP server when available. Start with `bilge_scan` for a dry run. Use `bilge_list_targets` to discover platform-specific targets, then scan a narrower task or target when that will make the result easier to act on.

Summarize the largest measured cache findings with their paths and sizes. Distinguish measured directory sizes from commands that have no reliable savings estimate, such as Docker prune. Mention partial scans and errors; do not promise that every reported byte will be reclaimed.

Before calling `bilge_clean`, identify the exact task, target, and scope to the user and obtain their authorization. Set `confirm=true` only after that authorization. Prefer the narrowest useful cleanup. Do not set `unsafe_clean=true` unless the user explicitly authorizes bypassing Bilge's Git safety check. Never treat a dry-run result as permission to clean.

After cleanup, call `bilge_scan` again for the same scope and report the observed change. If the MCP server is unavailable, tell the user and use the installed Bilge CLI only when appropriate; do not silently run cleanup through another route.
