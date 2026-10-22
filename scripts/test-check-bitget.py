#!/usr/bin/env python3
"""Check diagnostic output without Docker, real credentials, or network access."""
from contextlib import redirect_stdout
import hashlib
import hmac
import base64
import importlib.util
import io
import json
import sys
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import urllib.error

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("check_bitget", Path(__file__).with_name("check-bitget.py"))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class DiagnosticTests(unittest.TestCase):
    def run_check(self, code="40038", status=400, failure=False):
        env = ["BITGET_API_KEY=private-key", "BITGET_API_SECRET=private-secret",
               "BITGET_PASSPHRASE=private-pass", "BITGET_ACCOUNT_TYPE=uta"]
        requests = []

        class Response(io.BytesIO):
            pass

        def open_request(request, timeout):
            requests.append(request)
            if failure:
                raise ValueError("private-key private-secret private-pass")
            body = json.dumps({"code": code, "msg": "private-key private-secret private-pass",
                               "data": {"assets": [{"coin": "private-holding", "balance": "999"}]}}).encode()
            if status != 200:
                raise urllib.error.HTTPError(request.full_url, status, "private-message", {}, io.BytesIO(body))
            response = Response(body)
            response.code = status
            return response

        output = io.StringIO()
        with patch.object(checker.subprocess, "run", return_value=SimpleNamespace(stdout=json.dumps(env))), \
             patch.object(checker.urllib.request, "build_opener", return_value=SimpleNamespace(open=open_request)), \
             patch.object(checker.time, "time", return_value=1700000000), redirect_stdout(output):
            status = checker.main()
        text = output.getvalue()
        self.assertNotIn("private-", text)
        self.assertNotIn("999", text)
        return status, [json.loads(line) for line in text.splitlines()], requests

    def test_http_error_retains_provider_code_without_private_data(self):
        result, lines, requests = self.run_check()
        self.assertEqual(result, 1)
        self.assertEqual([line["bitget_code"] for line in lines[1:]], ["40038", "40038"])
        self.assertEqual(len(requests), 2)

    def test_read_only_headers_and_signatures(self):
        result, lines, requests = self.run_check("00000", 200)
        self.assertEqual(result, 0)
        self.assertTrue(all(line["ok"] for line in lines[1:]))
        for request in requests:
            self.assertEqual(request.get_method(), "GET")
            self.assertEqual(request.get_header("Content-type"), "application/json")
            self.assertTrue(request.full_url.startswith("https://api.bitget.com/api/v3/"))
            path = request.full_url.removeprefix("https://api.bitget.com")
            expected = base64.b64encode(hmac.new(b"private-secret", ("1700000000000GET" + path).encode(), hashlib.sha256).digest()).decode()
            self.assertEqual(request.get_header("Access-sign"), expected)

    def test_untrusted_codes_and_transport_errors_do_not_leak(self):
        _, lines, _ = self.run_check("private-code")
        self.assertEqual(lines[1]["bitget_code"], "unavailable")
        result, lines, _ = self.run_check(failure=True)
        self.assertEqual(result, 1)
        self.assertEqual(lines[1]["error"], "connection_failed")

    def test_redirect_is_not_followed(self):
        self.assertIsNone(checker.NoRedirect().redirect_request(None, None, 302, "", {}, "https://other.example"))


if __name__ == "__main__":
    unittest.main()
