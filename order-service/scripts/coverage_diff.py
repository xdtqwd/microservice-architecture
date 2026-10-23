#!/usr/bin/env python3
import subprocess
import sys
import re

THRESHOLD = 80
MODULE = "order-service"

def get_diff_lines():
    result = subprocess.run(
        ["git", "diff", "--unified=0", "origin/main...HEAD", "--", "*.go"],
        capture_output=True, text=True
    )
    files = {}
    current_file = None
    for line in result.stdout.splitlines():
        if line.startswith("+++ b/"):
            path = line[6:]
            if path.endswith("_test.go") or "vendor/" in path:
                current_file = None
            else:
                current_file = path
                files.setdefault(current_file, set())
        elif line.startswith("@@ ") and current_file:
            m = re.search(r'\+(\d+)(?:,(\d+))?', line)
            if m:
                start = int(m.group(1))
                count = int(m.group(2)) if m.group(2) is not None else 1
                for i in range(start, start + count):
                    files[current_file].add(i)
    return files

def parse_coverage(coverage_file):
    coverage = {}
    with open(coverage_file) as f:
        for line in f:
            line = line.strip()
            if line.startswith("mode:") or not line:
                continue
            m = re.match(r'^(.+):(\d+)\.\d+,(\d+)\.\d+\s+(\d+)\s+(\d+)$', line)
            if not m:
                continue
            filepath, start, end, stmts, hits = m.groups()
            if filepath.startswith(MODULE + "/"):
                filepath = filepath[len(MODULE)+1:]
            for lineno in range(int(start), int(end)+1):
                coverage.setdefault(filepath, {})[lineno] = int(hits)
    return coverage

def main():
    diff_lines = get_diff_lines()
    if not diff_lines:
        print("No Go files changed. Coverage diff: N/A (pass)")
        sys.exit(0)

    try:
        coverage = parse_coverage("coverage.out")
    except FileNotFoundError:
        print("coverage.out not found. Run: go test -coverprofile=coverage.out ./...")
        sys.exit(1)

    total_coverable = 0
    total_covered = 0
    uncovered = []

    for filepath, lines in diff_lines.items():
        file_cov = coverage.get(filepath, {})
        for lineno in sorted(lines):
            if lineno not in file_cov:
                continue
            total_coverable += 1
            if file_cov[lineno] > 0:
                total_covered += 1
            else:
                uncovered.append(f"{filepath}:{lineno}")

    if total_coverable == 0:
        print("No coverable lines in diff. Pass.")
        sys.exit(0)

    pct = total_covered / total_coverable * 100
    print(f"Diff coverage: {total_covered}/{total_coverable} lines = {pct:.1f}%")

    if uncovered:
        print(f"\nUncovered lines ({len(uncovered)}):")
        for line in uncovered:
            print(f"  {line}")

    if pct < THRESHOLD:
        print(f"\nFAIL: {pct:.1f}% < {THRESHOLD}% threshold")
        sys.exit(1)
    else:
        print(f"\nPASS: {pct:.1f}% >= {THRESHOLD}%")
        sys.exit(0)

if __name__ == "__main__":
    main()
