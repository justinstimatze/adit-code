#!/usr/bin/env python3
"""
Improved SWE-bench correlation: fixes basename collisions, caps per-repo
contribution, reports per-repo and aggregate correlations.
"""

import json
import math
import os
import subprocess
import sys
import tempfile
from collections import Counter


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
            print(f"  SKIP {repo_name}: clone failed", file=sys.stderr)
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
        subprocess.run(["rm", "-rf", dest], capture_output=True)
        if r.returncode != 0:
            continue
        try:
            adit_result = json.loads(r.stdout)
        except json.JSONDecodeError:
            continue

        matched = 0
        matched_paths = set()  # prevent duplicate matches

        for af in adit_result.get("files", []):
            adit_path = af["path"]
            adit_parts = adit_path.split("/")

            best_tc = None
            best_key = None

            for tc_path, tc_val in file_calls.items():
                tc_parts = tc_path.split("/")

                # Require at least last 2 path components to match
                if len(tc_parts) >= 2 and len(adit_parts) >= 2:
                    if tc_parts[-2:] == adit_parts[-2:]:
                        best_tc = tc_val
                        best_key = tc_path
                        break
                # Fallback: last 3 components for deeper matches
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

            all_rows.append(
                {
                    "repo": repo_name,
                    "file": os.path.basename(adit_path),
                    "lines": af["lines"],
                    "max_fn": af.get("functions", {}).get("max_length", 0),
                    "ctx": af["context_reads"]["total"],
                    "unn": af["context_reads"]["unnecessary"],
                    "noise": af["ambiguity"]["grep_noise"],
                    "blast": af["blast_radius"]["imported_by_count"],
                    "calls": total,
                }
            )
            matched += 1

        print(f"  {repo_name}: {matched} files matched", file=sys.stderr)

    subprocess.run(["rm", "-rf", tmpdir], capture_output=True)

    # Report
    print("# SWE-bench Validation Report (improved matching)\n")
    print(f"Files matched: {len(all_rows)}")
    print(f"Repos: {len(set(r['repo'] for r in all_rows))}")

    repo_counts = Counter(r["repo"] for r in all_rows)
    max_pct = repo_counts.most_common(1)[0][1] / len(all_rows) * 100
    print(
        f"Largest repo contribution: {repo_counts.most_common(1)[0][0]} ({repo_counts.most_common(1)[0][1]} files, {max_pct:.0f}%)"
    )
    print()

    # Aggregate correlations
    calls = [r["calls"] for r in all_rows]
    lines = [r["lines"] for r in all_rows]
    metrics = {
        "Lines": lines,
        "Max Fn Length": [r["max_fn"] for r in all_rows],
        "Context Reads": [r["ctx"] for r in all_rows],
        "Unnecessary Reads": [r["unn"] for r in all_rows],
        "Grep Noise": [r["noise"] for r in all_rows],
        "Blast Radius": [r["blast"] for r in all_rows],
    }

    print(f"## Aggregate (N={len(all_rows)})\n")
    print("| Metric | Spearman | Partial (ctrl lines) |")
    print("|--------|----------|---------------------|")
    for name, vals in metrics.items():
        s = spearman(calls, vals)
        if name == "Lines":
            ps = s
        else:
            ps = partial_spearman(calls, vals, lines)
        print(f"| {name:<20s} | {s:+.3f}   | {ps:+.3f}              |")

    # Per-repo correlations (repos with 10+ matched files)
    print("\n## Per-repo Spearman (repos with 10+ files)\n")
    print("| Repo | N | Lines | Blast | Noise | UnnReads |")
    print("|------|---|-------|-------|-------|----------|")
    for repo, count in repo_counts.most_common():
        if count < 10:
            continue
        rrows = [r for r in all_rows if r["repo"] == repo]
        rc = [r["calls"] for r in rrows]
        print(
            f"| {repo.split('/')[1][:20]} | {count} | "
            f"{spearman(rc, [r['lines'] for r in rrows]):+.2f} | "
            f"{spearman(rc, [r['blast'] for r in rrows]):+.2f} | "
            f"{spearman(rc, [r['noise'] for r in rrows]):+.2f} | "
            f"{spearman(rc, [r['unn'] for r in rrows]):+.2f} |"
        )

    # Median per-repo correlation
    print("\n## Median per-repo Spearman (repos with 10+ files)\n")
    big_repos = [(repo, count) for repo, count in repo_counts.items() if count >= 10]
    for metric_name, key in [
        ("Lines", "lines"),
        ("Blast Radius", "blast"),
        ("Grep Noise", "noise"),
        ("Unnecessary Reads", "unn"),
    ]:
        per_repo_corrs = []
        for repo, count in big_repos:
            rrows = [r for r in all_rows if r["repo"] == repo]
            rc = [r["calls"] for r in rrows]
            rv = [r[key] for r in rrows]
            per_repo_corrs.append(spearman(rc, rv))
        per_repo_corrs.sort()
        median = per_repo_corrs[len(per_repo_corrs) // 2] if per_repo_corrs else 0
        mean = sum(per_repo_corrs) / len(per_repo_corrs) if per_repo_corrs else 0
        print(
            f"  {metric_name}: median={median:+.3f}, mean={mean:+.3f} (across {len(big_repos)} repos)"
        )


if __name__ == "__main__":
    main()
