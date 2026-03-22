#!/usr/bin/env python3
"""
Test ALL metrics (existing + new candidates) against SWE-bench data.

New metrics from literature review:
- Max nesting depth (LM-CC paper: nesting > linear length for LLMs)
- Definition count (catalogs vs monoliths)
- Definition density (definitions per 100 lines)
- Import fan-out weighted by target file size
"""

import json
import math
import os
import subprocess
import sys
import tempfile
from collections import Counter

VENV = "/tmp/swe-bench-venv"
for v in ["python3.13", "python3.12", "python3.11"]:
    p = os.path.join(VENV, "lib", v, "site-packages")
    if os.path.exists(p):
        sys.path.insert(0, p)

import tree_sitter
import tree_sitter_python


def compute_max_nesting(filepath):
    """Compute max nesting depth using tree-sitter."""
    try:
        parser = tree_sitter.Parser(tree_sitter.Language(tree_sitter_python.language()))
        with open(filepath, "rb") as f:
            src = f.read()
        tree = parser.parse(src)

        max_depth = 0

        def walk(node, depth):
            nonlocal max_depth
            # Count nesting-inducing nodes
            if node.type in (
                "if_statement",
                "for_statement",
                "while_statement",
                "try_statement",
                "with_statement",
                "function_definition",
                "class_definition",
                "elif_clause",
                "else_clause",
                "except_clause",
                "finally_clause",
                "match_statement",
                "case_clause",
            ):
                depth += 1
                if depth > max_depth:
                    max_depth = depth
            for child in node.children:
                walk(child, depth)

        walk(tree.root_node, 0)
        return max_depth
    except Exception:
        return 0


def pearson(x, y):
    n = len(x)
    if n < 3:
        return 0.0
    sx, sy = sum(x), sum(y)
    sxy = sum(a * b for a, b in zip(x, y))
    sx2, sy2 = sum(a * a for a in x), sum(b * b for b in y)
    num = n * sxy - sx * sy
    den = math.sqrt((n * sx2 - sx * sx) * (n * sy2 - sy * sy))
    return num / den if den else 0.0


def spearman(x, y):
    def ranks(d):
        idx = sorted(range(len(d)), key=lambda i: d[i])
        r = [0.0] * len(d)
        for rank, i in enumerate(idx):
            r[i] = float(rank + 1)
        return r

    if len(x) < 3:
        return 0.0
    return pearson(ranks(x), ranks(y))


def partial_spearman(x, y, z):
    rxy = spearman(x, y)
    rxz = spearman(x, z)
    ryz = spearman(y, z)
    num = rxy - rxz * ryz
    den = math.sqrt((1 - rxz**2) * (1 - ryz**2))
    return num / den if den > 0 else 0.0


def main():
    max_repos = 30
    for arg in sys.argv[1:]:
        if arg.startswith("--repos="):
            max_repos = int(arg.split("=")[1])

    adit = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "adit")
    with open("/tmp/swe-bench-file-calls.json") as f:
        data = json.load(f)

    repos = sorted(
        data["repo_file_calls"].items(),
        key=lambda r: sum(f["reads"] + f["edits"] for f in r[1].values()),
        reverse=True,
    )[:max_repos]

    all_rows = []
    tmpdir = tempfile.mkdtemp()

    for repo_name, file_calls in repos:
        dest = os.path.join(tmpdir, repo_name.replace("/", "_"))
        r = subprocess.run(
            [
                "git",
                "clone",
                "--depth=1",
                "-q",
                f"https://github.com/{repo_name}.git",
                dest,
            ],
            capture_output=True,
            timeout=60,
        )
        if r.returncode != 0:
            print(f"  SKIP {repo_name}", file=sys.stderr)
            continue

        src_dir = dest
        for c in [
            os.path.join(dest, repo_name.split("/")[1].replace("-", "_")),
            os.path.join(dest, "src"),
            os.path.join(dest, "lib"),
        ]:
            if os.path.isdir(c):
                src_dir = c
                break

        r = subprocess.run(
            [adit, "score", src_dir], capture_output=True, text=True, timeout=120
        )
        if r.returncode != 0:
            subprocess.run(["rm", "-rf", dest], capture_output=True)
            continue
        try:
            adit_result = json.loads(r.stdout)
        except json.JSONDecodeError:
            subprocess.run(["rm", "-rf", dest], capture_output=True)
            continue

        matched = 0
        matched_paths = set()

        for af in adit_result.get("files", []):
            adit_path = af["path"]
            adit_parts = adit_path.split("/")
            base = os.path.basename(adit_path)

            best_tc = None
            best_key = None
            for tc_path, tc_val in file_calls.items():
                tc_parts = tc_path.split("/")
                if len(tc_parts) >= 2 and len(adit_parts) >= 2:
                    if tc_parts[-2:] == adit_parts[-2:]:
                        best_tc = tc_val
                        best_key = tc_path
                        break
                if len(tc_parts) >= 3 and len(adit_parts) >= 3:
                    if tc_parts[-3:] == adit_parts[-3:]:
                        best_tc = tc_val
                        best_key = tc_path
                        break

            if not best_tc or best_key in matched_paths:
                continue
            matched_paths.add(best_key)

            total = best_tc["reads"] + best_tc["edits"]
            if total == 0:
                continue

            # Compute new metrics
            nesting = 0
            if adit_path.endswith(".py") and os.path.exists(adit_path):
                nesting = compute_max_nesting(adit_path)

            fn_count = af.get("functions", {}).get("count", 0)
            fn_avg = af.get("functions", {}).get("avg_length", 0)
            def_count = len([d for d in []])  # we'll use ambiguity total as proxy
            lines = af["lines"]
            def_density = fn_count / max(lines, 1) * 100  # definitions per 100 lines

            all_rows.append(
                {
                    "repo": repo_name,
                    "file": base,
                    "lines": lines,
                    "max_fn": af.get("functions", {}).get("max_length", 0),
                    "fn_count": fn_count,
                    "fn_avg": fn_avg,
                    "def_density": def_density,
                    "nesting": nesting,
                    "ctx": af["context_reads"]["total"],
                    "unn": af["context_reads"]["unnecessary"],
                    "noise": af["ambiguity"]["grep_noise"],
                    "blast": af["blast_radius"]["imported_by_count"],
                    "calls": total,
                }
            )
            matched += 1

        print(f"  {repo_name}: {matched} files", file=sys.stderr)
        subprocess.run(["rm", "-rf", dest], capture_output=True)

    subprocess.run(["rm", "-rf", tmpdir], capture_output=True)

    print("\n# All Metrics vs SWE-bench Agent Tool Calls\n")
    print(
        f"N = {len(all_rows)} files across {len(set(r['repo'] for r in all_rows))} repos\n"
    )

    calls = [r["calls"] for r in all_rows]
    lines = [r["lines"] for r in all_rows]

    metrics = [
        ("Lines", [r["lines"] for r in all_rows]),
        ("Max Fn Length", [r["max_fn"] for r in all_rows]),
        ("Fn Count", [r["fn_count"] for r in all_rows]),
        ("Fn Avg Length", [r["fn_avg"] for r in all_rows]),
        ("Def Density (/100 lines)", [r["def_density"] for r in all_rows]),
        ("Max Nesting Depth", [r["nesting"] for r in all_rows]),
        ("Context Reads", [r["ctx"] for r in all_rows]),
        ("Unnecessary Reads", [r["unn"] for r in all_rows]),
        ("Grep Noise", [r["noise"] for r in all_rows]),
        ("Blast Radius", [r["blast"] for r in all_rows]),
    ]

    print("## Aggregate correlations\n")
    print("| Metric | Spearman | Partial (ctrl lines) | Category |")
    print("|--------|----------|---------------------|----------|")
    for name, vals in metrics:
        s = spearman(calls, vals)
        if name == "Lines":
            ps = s
            cat = "baseline"
        else:
            ps = partial_spearman(calls, vals, lines)
            if abs(ps) > 0.15:
                cat = "independent signal"
            elif abs(ps) > 0.05:
                cat = "weak independent"
            else:
                cat = "size proxy"
        print(f"| {name:<25s} | {s:+.3f}   | {ps:+.3f}              | {cat} |")

    # Median per-repo
    print("\n## Median per-repo Spearman (repos with 10+ files)\n")
    repo_counts = Counter(r["repo"] for r in all_rows)
    big_repos = [(repo, count) for repo, count in repo_counts.items() if count >= 10]
    print(f"Repos with 10+ files: {len(big_repos)}\n")

    print("| Metric | Median | Mean | Positive repos |")
    print("|--------|--------|------|----------------|")
    for name, key in [
        ("Lines", "lines"),
        ("Max Nesting", "nesting"),
        ("Grep Noise", "noise"),
        ("Blast Radius", "blast"),
        ("Fn Count", "fn_count"),
        ("Def Density", "def_density"),
        ("Unnecessary Reads", "unn"),
        ("Context Reads", "ctx"),
        ("Max Fn Length", "max_fn"),
        ("Fn Avg Length", "fn_avg"),
    ]:
        corrs = []
        for repo, count in big_repos:
            rrows = [r for r in all_rows if r["repo"] == repo]
            rc = [r["calls"] for r in rrows]
            rv = [r[key] for r in rrows]
            corrs.append(spearman(rc, rv))
        corrs.sort()
        median = corrs[len(corrs) // 2] if corrs else 0
        mean = sum(corrs) / len(corrs) if corrs else 0
        pos = sum(1 for c in corrs if c > 0)
        print(f"| {name:<25s} | {median:+.3f} | {mean:+.3f} | {pos}/{len(corrs)} |")


if __name__ == "__main__":
    main()
