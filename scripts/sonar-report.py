"""Export the scanner's analysis, including a failed gate, without saving credentials."""

import json
import os
import sys
import time
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urlencode, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # A redirect must never forward the Sonar token to another service.
        return None


def export_report(base, token, project, directory):
    url = urlsplit(base)
    if (url.scheme not in ("http", "https") or not url.hostname
            or url.username or url.password or url.query or url.fragment):
        raise ValueError("Invalid SONAR_HOST_URL")
    if url.scheme == "http" and url.hostname not in ("localhost", "127.0.0.1", "::1"):
        raise ValueError("Sonar HTTP is allowed only on loopback; use HTTPS elsewhere")
    opener = build_opener(NoRedirect)

    def get(endpoint, **params):
        request = Request(base.rstrip("/") + "/api/" + endpoint + "?" + urlencode(params),
                          headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
        with opener.open(request, timeout=30) as response:
            return json.load(response)

    def save(name, data):
        (directory / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    metadata = dict(line.split("=", 1) for line in
                    (directory / "report-task.txt").read_text(encoding="utf-8").splitlines() if "=" in line)
    if metadata.get("projectKey") != project or not metadata.get("ceTaskId"):
        raise ValueError("Scanner metadata does not match the requested project")
    # Only use the task ID; ignore URLs supplied in report-task.txt.
    for _ in range(60):
        task = get("ce/task", id=metadata["ceTaskId"])["task"]
        if task["status"] not in ("PENDING", "IN_PROGRESS"):
            break
        time.sleep(5)
    if task["status"] != "SUCCESS" or task.get("componentKey") != project:
        raise ValueError("Sonar analysis did not complete successfully")
    analysis_id = task["analysisId"]
    save("analysis.json", {"projectKey": project, "analysisId": analysis_id,
                           "ceTaskId": metadata["ceTaskId"], "revision": os.environ.get("GITHUB_SHA"),
                           "projectSnapshots": ["measures.json", "issues.json"]})
    gate = get("qualitygates/project_status", analysisId=analysis_id)
    save("quality-gate.json", gate)

    # shortcut: project snapshots rely on serialized CI; use a Browse-capable token
    # and check project_analyses before/after export if external concurrent scans are introduced.
    save("measures.json", get("measures/component", component=project,
                             metricKeys="bugs,vulnerabilities,code_smells,security_hotspots,coverage,"
                             "new_coverage,duplicated_lines_density,new_duplicated_lines_density,ncloc"))
    items = []
    page = 1
    while True:
        data = get("issues/search", componentKeys=project, resolved="false", p=page, ps=500)
        items.extend({k: v for k, v in item.items() if k not in ("author", "assignee", "hash", "flows")}
                     for item in data["issues"])
        total = data["paging"]["total"]
        if len(items) >= total:
            break
        if not data["issues"]:
            raise ValueError("Incomplete Sonar pagination")
        page += 1
    save("issues.json", {"exportedAfterAnalysisId": analysis_id, "total": total, "issues": items})
    print("Sonar JSON exported; Quality Gate: " + gate["projectStatus"]["status"])
    # shortcut: gate is report-only by user approval; restore enforcement after remediation.
    if gate["projectStatus"]["status"] != "OK":
        print("::warning::Sonar Quality Gate did not pass; report-only mode is enabled.")
    return 0


def main():
    directory = Path("reports/ci/sonar")
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "export-error.json").unlink(missing_ok=True)
    try:
        return export_report(os.environ["SONAR_HOST_URL"], os.environ["SONAR_TOKEN"],
                             os.environ["SONAR_PROJECT_KEY"], directory)
    except (OSError, ValueError, KeyError) as error:
        # Do not serialize response bodies, URLs or environment values on failures.
        detail = "HTTP " + str(error.code) if isinstance(error, HTTPError) else type(error).__name__
        (directory / "export-error.json").write_text(json.dumps({"error": detail}) + "\n", encoding="utf-8")
        print("Sonar JSON export failed: " + detail, file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
