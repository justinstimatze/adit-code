#!/usr/bin/env python3
"""
Correlate adit metrics with SWE-bench agent tool call counts.

Streams the nebius/SWE-rebench-openhands-trajectories dataset,
extracts per-file tool call counts, then runs adit on the same repos
to correlate structural metrics with actual agent behavior.

Usage:
    python3 cmd/swe-bench-validate/main.py [--limit N] [--resolved-only]
"""

import json
import os
import sys
from collections import defaultdict

# Add venv to path
VENV = "/tmp/swe-bench-venv"
if os.path.exists(VENV):
    sys.path.insert(0, os.path.join(VENV, "lib", "python3.13", "site-packages"))
    # Try other Python versions
    for v in ["python3.12", "python3.11", "python3.10"]:
        p = os.path.join(VENV, "lib", v, "site-packages")
        if os.path.exists(p):
            sys.path.insert(0, p)

from datasets import load_dataset


def extract_file_tool_calls(trajectory):
    """Extract per-file Read/Grep/Edit counts from a trajectory."""
    file_reads = defaultdict(int)
    file_greps = defaultdict(int)
    file_edits = defaultdict(int)
    total_reads = 0
    total_greps = 0
    total_edits = 0

    for msg in trajectory:
        if msg.get("role") != "assistant":
            continue
        tool_calls = msg.get("tool_calls")
        if not tool_calls:
            continue

        for tc in tool_calls:
            fn = tc.get("function", {})
            name = fn.get("name", "")
            try:
                args = json.loads(fn.get("arguments", "{}"))
            except (json.JSONDecodeError, TypeError):
                continue

            if name in ("str_replace_editor", "str_replace_based_edit_tool"):
                cmd = args.get("command", "")
                path = args.get("path", "")
                if not path or path == "/workspace":
                    continue
                # Normalize path: strip /workspace/repo__name/ prefix
                parts = path.split("/")
                if len(parts) > 2 and parts[1] == "workspace":
                    path = "/".join(parts[3:])  # skip /workspace/repo__ver/
                if cmd == "view":
                    file_reads[path] += 1
                    total_reads += 1
                elif cmd in ("str_replace", "insert"):
                    file_edits[path] += 1
                    total_edits += 1
            elif name == "bash":
                cmd_str = args.get("command", "")
                if any(g in cmd_str for g in ["grep ", "rg ", " find ", "ag "]):
                    total_greps += 1
                elif any(r in cmd_str for r in ["cat ", "head ", "tail "]):
                    total_reads += 1

    return {
        "file_reads": dict(file_reads),
        "file_edits": dict(file_edits),
        "total_reads": total_reads,
        "total_greps": total_greps,
        "total_edits": total_edits,
    }


def main():
    limit = 500  # Process N trajectories
    resolved_only = False

    for arg in sys.argv[1:]:
        if arg.startswith("--limit="):
            limit = int(arg.split("=")[1])
        elif arg == "--resolved-only":
            resolved_only = True

    print(
        f"Loading SWE-rebench-openhands-trajectories (limit={limit})...",
        file=sys.stderr,
    )

    ds = load_dataset(
        "nebius/SWE-rebench-openhands-trajectories",
        split="train",
        streaming=True,
    )

    # Aggregate per-file tool calls across all trajectories for each repo
    # Key: (repo, relative_file_path) -> {reads, edits, greps}
    repo_file_calls = defaultdict(
        lambda: defaultdict(lambda: {"reads": 0, "edits": 0, "greps": 0})
    )
    repo_count = defaultdict(int)
    processed = 0
    skipped = 0

    for row in ds:
        if processed >= limit:
            break

        if resolved_only and not row.get("resolved"):
            skipped += 1
            continue

        repo = row.get("repo", "")
        if not repo:
            skipped += 1
            continue

        tc = extract_file_tool_calls(row["trajectory"])
        if tc["total_reads"] == 0 and tc["total_edits"] == 0:
            skipped += 1
            continue

        for path, count in tc["file_reads"].items():
            repo_file_calls[repo][path]["reads"] += count
        for path, count in tc["file_edits"].items():
            repo_file_calls[repo][path]["edits"] += count

        repo_count[repo] += 1
        processed += 1

        if processed % 100 == 0:
            print(
                f"  Processed {processed} trajectories, {len(repo_file_calls)} repos...",
                file=sys.stderr,
            )

    print(
        f"Processed {processed} trajectories across {len(repo_file_calls)} repos (skipped {skipped})",
        file=sys.stderr,
    )

    # Find repos with the most data
    repos_by_files = sorted(
        repo_file_calls.items(), key=lambda x: len(x[1]), reverse=True
    )

    print("\nTop repos by files touched:")
    for repo, files in repos_by_files[:20]:
        total_reads = sum(f["reads"] for f in files.values())
        total_edits = sum(f["edits"] for f in files.values())
        print(
            f"  {repo}: {len(files)} files, {total_reads} reads, {total_edits} edits, {repo_count[repo]} trajectories"
        )

    # Output aggregated data as JSON for the Go tool to consume
    output = {
        "trajectories_processed": processed,
        "repos": len(repo_file_calls),
        "repo_file_calls": {
            repo: {path: calls for path, calls in files.items()}
            for repo, files in repos_by_files[:50]  # top 50 repos
        },
    }

    output_path = "/tmp/swe-bench-file-calls.json"
    with open(output_path, "w") as f:
        json.dump(output, f, indent=2)
    print(f"\nWrote {output_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
