#!/usr/bin/env python3

# Copyright © 2026 SUSE LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Render a Ginkgo JUnit report as Markdown for $GITHUB_STEP_SUMMARY."""

import html
import sys
import xml.etree.ElementTree as ET

MAX_FAILURE_LINES = 60
ICONS = {"passed": "✅", "skipped": "⏭️", "pending": "⏭️"}


def duration(seconds):
    seconds = int(float(seconds or 0))
    return f"{seconds // 60}m {seconds % 60:02d}s"


def main(path, title):
    try:
        root = ET.parse(path).getroot()
    except (OSError, ET.ParseError) as err:
        print(f"### {title}\n\nNo JUnit report found at `{path}`: {err}\n")
        return

    cases = root.iter("testcase")
    rows, failures, counts = [], [], {}
    for case in cases:
        name = case.get("name", "")
        status = case.get("status") or ("failed" if case.find("failure") is not None else "passed")
        # Suite setup and teardown nodes only matter when they fail.
        if not name.startswith("[It]") and status == "passed":
            continue
        counts[status] = counts.get(status, 0) + 1
        cell = html.escape(name).replace("|", "\\|")
        rows.append(f"| {ICONS.get(status, '❌')} {status} | {cell} | {duration(case.get('time'))} |")
        failure = case.find("failure")
        if failure is not None:
            text = (failure.text or "").strip().splitlines()
            if len(text) > MAX_FAILURE_LINES:
                text = text[:MAX_FAILURE_LINES] + [f"... {len(text) - MAX_FAILURE_LINES} more lines in the artifacts"]
            failures.append(
                f"<details><summary>{html.escape(name)}: {html.escape(failure.get('message', ''))}</summary>\n\n"
                f"<pre>{html.escape(chr(10).join(text))}</pre>\n</details>\n"
            )

    failed = sum(n for s, n in counts.items() if s not in ICONS)
    totals = ", ".join(f"{n} {s}" for s, n in sorted(counts.items()))
    print(f"### {title}: {'❌ failed' if failed else '✅ passed'}\n")
    print(f"{totals}. Total time {duration(root.get('time'))}.\n")
    print("| Result | Spec | Duration |\n|---|---|---|")
    print("\n".join(rows) + "\n")
    if failures:
        print("#### Failures\n")
        print("\n".join(failures))


if __name__ == "__main__":
    if len(sys.argv) not in (2, 3):
        sys.exit(f"usage: {sys.argv[0]} JUNIT_XML [TITLE]")
    main(sys.argv[1], sys.argv[2] if len(sys.argv) == 3 else "E2E results")
