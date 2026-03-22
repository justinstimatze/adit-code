#!/usr/bin/env python3
"""
Step 2: Clone repos, run adit, correlate with SWE-bench tool call data.

Reads /tmp/swe-bench-file-calls.json (from main.py), clones top repos,
runs adit, and computes Pearson + Spearman correlations.

Usage:
    python3 cmd/swe-bench-validate/correlate.py [--repos N]
"""

import json
import math
import os
import subprocess
import sys
import tempfile


def clone_repo(repo, dest):
    """Clone a repo shallowly. Returns True on success."""
    url = f"https://github.com/{repo}.git"
    result = subprocess.run(
        ["git", "clone", "--depth=1", "-q", url, dest],
        capture_output=True,
        text=True,
        timeout=60,
    )
    return result.returncode == 0


def run_adit(path, adit_binary):
    """Run adit score on a path, return parsed JSON."""
    result = subprocess.run(
        [adit_binary, "score", path],
        capture_output=True,
        text=True,
        timeout=120,
    )
    if result.returncode != 0:
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return None


def pearson(x, y):
    if len(x) != len(y) or len(x) < 3:
        return 0.0
    n = len(x)
    sx = sum(x)
    sy = sum(y)
    sxy = sum(a * b for a, b in zip(x, y))
    sx2 = sum(a * a for a in x)
    sy2 = sum(b * b for b in y)
    num = n * sxy - sx * sy
    den = math.sqrt((n * sx2 - sx * sx) * (n * sy2 - sy * sy))
    return num / den if den != 0 else 0.0


def spearman(x, y):
    def ranks(data):
        indexed = sorted(enumerate(data), key=lambda p: p[1])
        r = [0.0] * len(data)
        for rank, (idx, _) in enumerate(indexed):
            r[idx] = float(rank + 1)
        return r

    if len(x) != len(y) or len(x) < 3:
        return 0.0
    return pearson(ranks(x), ranks(y))


def main():
    max_repos = 20
    for arg in sys.argv[1:]:
        if arg.startswith("--repos="):
            max_repos = int(arg.split("=")[1])

    # Find adit binary
    adit = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "adit")
    if not os.path.exists(adit):
        # Try building it
        subprocess.run(
            ["go", "build", "-o", adit, "./cmd/adit"],
            cwd=os.path.dirname(os.path.abspath(__file__)) + "/../..",
        )
    if not os.path.exists(adit):
        print(
            "Error: adit binary not found. Run 'go build ./cmd/adit' first.",
            file=sys.stderr,
        )
        sys.exit(1)

    # Load tool call data
    tc_path = "/tmp/swe-bench-file-calls.json"
    if not os.path.exists(tc_path):
        print(f"Error: {tc_path} not found. Run main.py first.", file=sys.stderr)
        sys.exit(1)

    with open(tc_path) as f:
        data = json.load(f)

    repo_file_calls = data["repo_file_calls"]
    print(f"Loaded tool call data for {len(repo_file_calls)} repos", file=sys.stderr)

    # Sort repos by total tool calls
    repos = sorted(
        repo_file_calls.items(),
        key=lambda r: sum(f["reads"] + f["edits"] for f in r[1].values()),
        reverse=True,
    )[:max_repos]

    # Clone, score, correlate
    all_rows = []  # (lines, max_fn, context_reads, unnecessary, grep_noise, blast, tool_calls)

    with tempfile.TemporaryDirectory() as tmpdir:
        for repo_name, file_calls in repos:
            dest = os.path.join(tmpdir, repo_name.replace("/", "_"))
            print(f"Cloning {repo_name}...", file=sys.stderr)

            if not clone_repo(repo_name, dest):
                print(f"  Failed to clone {repo_name}", file=sys.stderr)
                continue

            # Find Python source directory
            src_dir = dest
            for candidate in [
                os.path.join(dest, repo_name.split("/")[1].replace("-", "_")),
                os.path.join(dest, "src"),
                os.path.join(dest, "lib"),
            ]:
                if os.path.isdir(candidate):
                    src_dir = candidate
                    break

            print(f"  Scoring {src_dir}...", file=sys.stderr)
            adit_result = run_adit(src_dir, adit)
            if not adit_result:
                print(f"  Failed to score {repo_name}", file=sys.stderr)
                # Clean up to save disk
                subprocess.run(["rm", "-rf", dest], capture_output=True)
                continue

            # Match adit file scores with tool call data
            matched = 0
            for adit_file in adit_result.get("files", []):
                adit_path = adit_file["path"]
                base = os.path.basename(adit_path)

                # Try to find matching tool call data
                # The tool call paths are relative to /workspace/repo/
                best_tc = None
                for tc_path_key, tc_val in file_calls.items():
                    if tc_path_key.endswith(base):
                        # Verify more of the path matches
                        tc_parts = tc_path_key.split("/")
                        adit_parts = adit_path.split("/")
                        # Check last 2 components
                        if len(tc_parts) >= 2 and len(adit_parts) >= 2:
                            if (
                                tc_parts[-2:] == adit_parts[-2:]
                                or tc_parts[-1] == adit_parts[-1]
                            ):
                                best_tc = tc_val
                                break
                        else:
                            best_tc = tc_val
                            break

                if not best_tc:
                    continue

                total_calls = best_tc["reads"] + best_tc["edits"]
                if total_calls == 0:
                    continue

                all_rows.append(
                    {
                        "repo": repo_name,
                        "file": base,
                        "lines": adit_file["lines"],
                        "max_fn": adit_file.get("functions", {}).get("max_length", 0),
                        "context_reads": adit_file["context_reads"]["total"],
                        "unnecessary": adit_file["context_reads"]["unnecessary"],
                        "grep_noise": adit_file["ambiguity"]["grep_noise"],
                        "blast_radius": adit_file["blast_radius"]["imported_by_count"],
                        "tool_calls": total_calls,
                    }
                )
                matched += 1

            print(f"  Matched {matched} files", file=sys.stderr)

            # Clean up clone
            subprocess.run(["rm", "-rf", dest], capture_output=True)

    # Compute correlations
    print("\n# SWE-bench Validation Report\n")
    print(f"Repos analyzed: {max_repos}")
    print(f"Files matched: {len(all_rows)}")
    print(f"Unique repos with matches: {len(set(r['repo'] for r in all_rows))}")
    print()

    if len(all_rows) < 10:
        print("Too few matched files for reliable correlations.")
        # Dump what we have
        for r in all_rows:
            print(
                f"  {r['repo']}/{r['file']}: {r['lines']} lines, {r['tool_calls']} calls"
            )
        return

    tool_calls = [r["tool_calls"] for r in all_rows]
    metrics = {
        "Lines": [r["lines"] for r in all_rows],
        "Max Function Length": [r["max_fn"] for r in all_rows],
        "Context Reads": [r["context_reads"] for r in all_rows],
        "Unnecessary Reads": [r["unnecessary"] for r in all_rows],
        "Grep Noise": [r["grep_noise"] for r in all_rows],
        "Blast Radius": [r["blast_radius"] for r in all_rows],
    }

    print("## Correlations: adit metrics vs SWE-bench agent tool calls\n")
    print(
        f"N = {len(all_rows)} files across {len(set(r['repo'] for r in all_rows))} repos\n"
    )
    print("| Metric | Pearson r | Spearman r | Interpretation |")
    print("|--------|-----------|------------|----------------|")

    for name, values in metrics.items():
        p = pearson(tool_calls, values)
        s = spearman(tool_calls, values)
        avg = (abs(p) + abs(s)) / 2
        if avg > 0.7:
            interp = "strong"
        elif avg > 0.4:
            interp = "moderate"
        elif avg > 0.2:
            interp = "weak"
        else:
            interp = "negligible"
        if p > 0:
            interp += " positive"
        elif p < 0:
            interp += " negative"
        print(f"| {name:<20s} | {p:+.3f}    | {s:+.3f}     | {interp:<20s} |")

    # Top files by tool calls
    print("\n## Top 15 files by tool calls\n")
    print("| Repo | File | Calls | Lines | MaxFn | Reads | Noise | Blast |")
    print("|------|------|-------|-------|-------|-------|-------|-------|")
    sorted_rows = sorted(all_rows, key=lambda r: -r["tool_calls"])
    for r in sorted_rows[:15]:
        print(
            f"| {r['repo'].split('/')[1][:15]} | {r['file'][:20]} | {r['tool_calls']} | {r['lines']} | {r['max_fn']} | {r['context_reads']} | {r['grep_noise']} | {r['blast_radius']} |"
        )


if __name__ == "__main__":
    main()
