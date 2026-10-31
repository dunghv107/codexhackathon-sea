"""Keep secret finding locations, never matched credentials or source snippets."""

import json
import sys


def redact(report):
    for result in report.get("Results", []):
        if "Secrets" in result:
            result["Secrets"] = [
                {key: finding[key] for key in ("RuleID", "Category", "Title", "Severity", "StartLine", "EndLine")
                 if key in finding}
                for finding in result["Secrets"]
            ]
    return report


if __name__ == "__main__":
    json.dump(redact(json.load(sys.stdin)), sys.stdout, indent=2)
    print()
