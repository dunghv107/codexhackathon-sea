import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlsplit


def module(name):
    path = Path(__file__).resolve().parents[2] / "scripts" / (name + ".py")
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


sonar = module("sonar-report")
trivy = module("redact-trivy")


class ReportTests(unittest.TestCase):
    def test_secret_report_preserves_location_but_removes_match_and_source(self):
        report = {"Results": [{"Secrets": [{"RuleID": "test", "StartLine": 9, "Severity": "HIGH",
                   "Match": "sensitive-value", "Code": {"Lines": [{"Content": "sensitive-value"}]}}],
                   "Vulnerabilities": [{"VulnerabilityID": "CVE-example"}]}]}
        redacted = trivy.redact(report)
        self.assertNotIn("sensitive-value", json.dumps(redacted))
        self.assertEqual(redacted["Results"][0]["Secrets"][0]["StartLine"], 9)
        self.assertEqual(len(redacted["Results"][0]["Vulnerabilities"]), 1)

    def run_export(self, gate="OK", project="project", pending=False):
        directory = Path(self.enterContext(tempfile.TemporaryDirectory()))
        (directory / "report-task.txt").write_text(
            "projectKey=project\nceTaskId=task-1\nceTaskUrl=https://untrusted.invalid/\n", encoding="utf-8")
        requests = []

        def open_request(request, timeout):
            url = urlsplit(request.full_url)
            self.assertEqual(url.netloc, "127.0.0.1:9000")
            self.assertEqual(request.get_header("Authorization"), "Bearer test-token")
            requests.append(request.full_url)
            params = parse_qs(url.query)
            if url.path.endswith("ce/task"):
                data = {"task": {"status": "IN_PROGRESS" if pending else "SUCCESS",
                                 "componentKey": "project", "analysisId": "analysis-1"}}
            elif url.path.endswith("project_status"):
                self.assertEqual(params["analysisId"], ["analysis-1"])
                data = {"projectStatus": {"status": gate}}
            elif url.path.endswith("issues/search"):
                data = {"paging": {"total": 2}, "issues": [{"key": params["p"][0], "author": "private"}]}
            else:
                data = {"component": {"measures": []}}
            return io.StringIO(json.dumps(data))

        with patch.object(sonar, "build_opener") as build, patch.object(sonar.time, "sleep"):
            build.return_value.open.side_effect = open_request
            result = sonar.export_report("http://127.0.0.1:9000", "test-token", project, directory)
        return result, directory, requests

    def test_failed_gate_warns_and_exports_original_status_without_blocking(self):
        with patch("sys.stdout", new_callable=io.StringIO) as output:
            status, directory, requests = self.run_export(gate="ERROR")
        self.assertEqual(status, 0)
        self.assertIn("::warning::", output.getvalue())
        gate = json.loads((directory / "quality-gate.json").read_text())
        self.assertEqual(gate["projectStatus"]["status"], "ERROR")
        data = json.loads((directory / "issues.json").read_text())
        self.assertEqual([item["key"] for item in data["issues"]], ["1", "2"])
        self.assertNotIn("private", json.dumps(data))
        self.assertTrue((directory / "measures.json").exists())
        self.assertTrue(any("p=2" in url for url in requests))

    def test_success_records_analysis_and_labels_project_snapshots(self):
        status, directory, _ = self.run_export()
        self.assertEqual(status, 0)
        metadata = json.loads((directory / "analysis.json").read_text())
        self.assertEqual(metadata["analysisId"], "analysis-1")
        self.assertEqual(metadata["projectSnapshots"], ["measures.json", "issues.json"])

    def test_wrong_project_and_unfinished_task_fail_closed(self):
        with self.assertRaisesRegex(ValueError, "metadata"):
            self.run_export(project="another-project")
        with self.assertRaisesRegex(ValueError, "did not complete"):
            self.run_export(pending=True)

    def test_credentials_cannot_follow_redirect_or_plaintext_remote_url(self):
        self.assertIsNone(sonar.NoRedirect().redirect_request(None, None, 302, "", {}, "https://elsewhere.invalid"))
        with self.assertRaisesRegex(ValueError, "loopback"):
            sonar.export_report("http://remote.invalid", "test-token", "project", Path("unused"))

    def test_http_error_does_not_serialize_body_url_or_token(self):
        with tempfile.TemporaryDirectory() as temporary, patch.object(sonar, "export_report") as export:
            error = HTTPError("https://private.invalid/", 401, "sensitive-value", {}, None)
            export.side_effect = error
            with patch.object(sonar, "Path", return_value=Path(temporary)), patch.dict(sonar.os.environ, {
                "SONAR_HOST_URL": "http://127.0.0.1:9000", "SONAR_TOKEN": "test-token", "SONAR_PROJECT_KEY": "project"
            }):
                self.assertEqual(sonar.main(), 1)
            self.assertEqual(json.loads((Path(temporary) / "export-error.json").read_text()), {"error": "HTTP 401"})
            error.close()


if __name__ == "__main__":
    unittest.main()
