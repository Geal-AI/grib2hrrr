#!/usr/bin/env python3
"""gen-test-report.py — Convert gotestsum JSON output to release-note markdown.

Usage:
    python3 .github/scripts/gen-test-report.py <test-output.json> <coverage.txt> <version>

Outputs markdown to stdout, suitable for appending to a GitHub release body.
"""

import json
import sys
from collections import defaultdict

def parse_tests(json_path):
    """Parse gotestsum JSON output into per-test results."""
    results = {}  # (package, test) -> {action, elapsed}
    package_counts = defaultdict(lambda: {"pass": 0, "fail": 0, "skip": 0})

    try:
        with open(json_path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    ev = json.loads(line)
                except json.JSONDecodeError:
                    continue

                pkg   = ev.get("Package", "")
                test  = ev.get("Test", "")
                action = ev.get("Action", "")

                if not test:
                    continue

                if action in ("pass", "fail", "skip"):
                    results[(pkg, test)] = {
                        "action":  action,
                        "elapsed": ev.get("Elapsed", 0.0),
                        "package": pkg,
                        "test":    test,
                    }
                    package_counts[pkg][action] += 1

    except FileNotFoundError:
        return {}, {}

    return results, package_counts


def parse_coverage(cov_path):
    """Extract total coverage line from go tool cover -func output."""
    try:
        with open(cov_path) as f:
            for line in f:
                if "total:" in line:
                    parts = line.split()
                    return parts[-1] if parts else "?"
    except FileNotFoundError:
        pass
    return "?"


def render(json_path, cov_path, version):
    results, pkg_counts = parse_tests(json_path)
    coverage = parse_coverage(cov_path)

    total_pass = sum(v["pass"] for v in pkg_counts.values())
    total_fail = sum(v["fail"] for v in pkg_counts.values())
    total_skip = sum(v["skip"] for v in pkg_counts.values())
    total = total_pass + total_fail + total_skip

    status_icon = "✅" if total_fail == 0 else "❌"

    lines = []
    lines.append(f"## {status_icon} Test Report — {version}")
    lines.append("")
    lines.append("| Metric | Value |")
    lines.append("|--------|-------|")
    lines.append(f"| Tests run | {total} |")
    lines.append(f"| ✅ Passed | {total_pass} |")
    if total_fail:
        lines.append(f"| ❌ Failed | {total_fail} |")
    if total_skip:
        lines.append(f"| ⏭ Skipped | {total_skip} |")
    lines.append(f"| Coverage | {coverage} |")
    lines.append("")

    # Package breakdown
    if pkg_counts:
        lines.append("### Package breakdown")
        lines.append("")
        lines.append("| Package | Pass | Fail | Skip |")
        lines.append("|---------|------|------|------|")
        for pkg, counts in sorted(pkg_counts.items()):
            icon = "✅" if counts["fail"] == 0 else "❌"
            short_pkg = pkg.split("/")[-1] or pkg
            lines.append(
                f"| {icon} `{short_pkg}` | {counts['pass']} | {counts['fail']} | {counts['skip']} |"
            )
        lines.append("")

    # List any failures
    failures = [r for r in results.values() if r["action"] == "fail"]
    if failures:
        lines.append("### ❌ Failing tests")
        lines.append("")
        for r in sorted(failures, key=lambda x: (x["package"], x["test"])):
            lines.append(f"- `{r['test']}` ({r['package']})")
        lines.append("")

    # Slowest tests (top 10, skipping sub-tests)
    slow = sorted(
        [r for r in results.values() if "/" not in r["test"] and r["elapsed"] > 0],
        key=lambda x: -x["elapsed"],
    )[:10]
    if slow:
        lines.append("### Slowest tests")
        lines.append("")
        lines.append("| Test | Time |")
        lines.append("|------|------|")
        for r in slow:
            lines.append(f"| `{r['test']}` | {r['elapsed']:.3f}s |")
        lines.append("")

    return "\n".join(lines)


if __name__ == "__main__":
    if len(sys.argv) < 4:
        print(f"Usage: {sys.argv[0]} <test-output.json> <coverage.txt> <version>",
              file=sys.stderr)
        sys.exit(1)

    print(render(sys.argv[1], sys.argv[2], sys.argv[3]))
