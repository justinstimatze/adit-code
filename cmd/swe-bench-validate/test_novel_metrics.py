#!/usr/bin/env python3
"""
Test novel metric candidates against SWE-bench data.

New candidates:
1. Import-to-definition ratio (wiring vs logic)
2. Comment density (context window waste)
3. Cross-file edit coupling (from SWE-bench patches)

Also re-tests existing metrics + nesting for comparison.
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


def compute_file_metrics(filepath):
    """Compute novel metrics from source file."""
    try:
        with open(filepath, "r", errors="replace") as f:
            lines = f.readlines()
    except Exception:
        return {}

    total_lines = len(lines)
    if total_lines == 0:
        return {}

    # Comment density: lines that are comments / total lines
    comment_lines = 0
    in_docstring = False
    for line in lines:
        stripped = line.strip()
        if stripped.startswith('"""') or stripped.startswith("'''"):
            if stripped.count('"""') >= 2 or stripped.count("'''") >= 2:
                comment_lines += 1  # single-line docstring
            else:
                in_docstring = not in_docstring
                comment_lines += 1
        elif in_docstring:
            comment_lines += 1
        elif stripped.startswith("#"):
            comment_lines += 1

    # Import lines
    import_lines = 0
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("import ") or stripped.startswith("from "):
            import_lines += 1

    # Blank lines
    blank_lines = sum(1 for line in lines if line.strip() == "")

    # Code lines (not blank, not comment, not import)
    code_lines = total_lines - comment_lines - blank_lines

    return {
        "import_ratio": import_lines / max(total_lines, 1),
        "comment_density": comment_lines / max(total_lines, 1),
        "code_density": code_lines / max(total_lines, 1),
        "import_lines": import_lines,
        "comment_lines": comment_lines,
        "blank_lines": blank_lines,
    }


def extract_edit_coupling(swe_bench_data):
    """From SWE-bench tool calls, find which files are edited together."""
    # For each trajectory, find all files that were edited
    # Then count co-edit pairs
    pass  # We'll compute this differently - from the trajectory data directly


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
    rxy, rxz, ryz = spearman(x, y), spearman(x, z), spearman(y, z)
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
            continue
        try:
            adit_result = json.loads(r.stdout)
        except json.JSONDecodeError:
            continue

        matched = 0
        matched_paths = set()

        for af in adit_result.get("files", []):
            adit_path = af["path"]
            adit_parts = adit_path.split("/")

            best_tc = None
            best_key = None
            for tc_path, tc_val in file_calls.items():
                tc_parts = tc_path.split("/")
                if (
                    len(tc_parts) >= 2
                    and len(adit_parts) >= 2
                    and tc_parts[-2:] == adit_parts[-2:]
                ):
                    best_tc = tc_val
                    best_key = tc_path
                    break
                if (
                    len(tc_parts) >= 3
                    and len(adit_parts) >= 3
                    and tc_parts[-3:] == adit_parts[-3:]
                ):
                    best_tc = tc_val
                    best_key = tc_path
                    break

            if not best_tc or best_key in matched_paths:
                continue
            matched_paths.add(best_key)
            total = best_tc["reads"] + best_tc["edits"]
            if total == 0:
                continue

            # Compute novel metrics on the actual file
            novel = {}
            if adit_path.endswith(".py") and os.path.exists(adit_path):
                novel = compute_file_metrics(adit_path)

            all_rows.append(
                {
                    "repo": repo_name,
                    "file": os.path.basename(adit_path),
                    "lines": af["lines"],
                    "nesting": af.get("max_nesting_depth", 0),
                    "max_fn": af.get("functions", {}).get("max_length", 0),
                    "fn_count": af.get("functions", {}).get("count", 0),
                    "ctx": af["context_reads"]["total"],
                    "unn": af["context_reads"]["unnecessary"],
                    "noise": af["ambiguity"]["grep_noise"],
                    "blast": af["blast_radius"]["imported_by_count"],
                    "import_ratio": novel.get("import_ratio", 0),
                    "comment_density": novel.get("comment_density", 0),
                    "code_density": novel.get("code_density", 0),
                    "import_lines": novel.get("import_lines", 0),
                    "calls": total,
                }
            )
            matched += 1

        print(f"  {repo_name}: {matched} files", file=sys.stderr)

    # Don't clean up tmpdir — keep shallow clones for inspection
    print(f"\n  Clones kept at {tmpdir}", file=sys.stderr)

    # Filter to files with novel metrics computed
    rows_with_novel = [r for r in all_rows if r["lines"] > 0]

    print("\n# All Metrics (Existing + Novel) vs SWE-bench\n")
    print(
        f"N = {len(rows_with_novel)} files across {len(set(r['repo'] for r in rows_with_novel))} repos\n"
    )

    calls = [r["calls"] for r in rows_with_novel]
    lines = [r["lines"] for r in rows_with_novel]

    all_metrics = [
        # Existing
        ("Lines", [r["lines"] for r in rows_with_novel]),
        ("Max Nesting Depth", [r["nesting"] for r in rows_with_novel]),
        ("Grep Noise", [r["noise"] for r in rows_with_novel]),
        ("Blast Radius", [r["blast"] for r in rows_with_novel]),
        ("Context Reads", [r["ctx"] for r in rows_with_novel]),
        ("Unnecessary Reads", [r["unn"] for r in rows_with_novel]),
        ("Max Fn Length", [r["max_fn"] for r in rows_with_novel]),
        ("Fn Count", [r["fn_count"] for r in rows_with_novel]),
        # Novel
        ("Import Ratio", [r["import_ratio"] for r in rows_with_novel]),
        ("Import Lines (abs)", [r["import_lines"] for r in rows_with_novel]),
        ("Comment Density", [r["comment_density"] for r in rows_with_novel]),
        ("Code Density", [r["code_density"] for r in rows_with_novel]),
    ]

    print("## Aggregate correlations\n")
    print("| Metric | Spearman | Partial (ctrl lines) | Category |")
    print("|--------|----------|---------------------|----------|")
    for name, vals in all_metrics:
        s = spearman(calls, vals)
        if name == "Lines":
            ps = s
            cat = "baseline"
        else:
            ps = partial_spearman(calls, vals, lines)
            if abs(ps) > 0.15:
                cat = "**independent**"
            elif abs(ps) > 0.05:
                cat = "weak independent"
            else:
                cat = "size proxy"
        print(f"| {name:<25s} | {s:+.3f}   | {ps:+.3f}              | {cat} |")

    # Median per-repo
    print("\n## Median per-repo Spearman (repos with 10+ files)\n")
    repo_counts = Counter(r["repo"] for r in rows_with_novel)
    big_repos = [(repo, count) for repo, count in repo_counts.items() if count >= 10]
    print(f"Repos with 10+ files: {len(big_repos)}\n")
    print("| Metric | Median | Mean | +/total |")
    print("|--------|--------|------|---------|")
    for name, key in [
        ("Lines", "lines"),
        ("Max Nesting", "nesting"),
        ("Grep Noise", "noise"),
        ("Blast Radius", "blast"),
        ("Context Reads", "ctx"),
        ("Unnecessary Reads", "unn"),
        ("Fn Count", "fn_count"),
        ("Max Fn Length", "max_fn"),
        ("Import Ratio", "import_ratio"),
        ("Import Lines", "import_lines"),
        ("Comment Density", "comment_density"),
        ("Code Density", "code_density"),
    ]:
        corrs = []
        for repo, count in big_repos:
            rrows = [r for r in rows_with_novel if r["repo"] == repo]
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
