import io
import os
import unittest
import urllib.error
from unittest import mock

from scripts import verify_real_provider as verifier


class RealProviderVerifierTests(unittest.TestCase):
    def test_redact_removes_explicit_and_credential_shaped_values(self):
        key = "sk-test-secret-123456789"
        got = verifier.redact(f"Bearer {key}; echoed={key}", (key,))
        self.assertNotIn(key, got)
        self.assertIn("[REDACTED]", got)

    @mock.patch.object(verifier.urllib.request, "urlopen")
    def test_http_error_omits_provider_body_and_key(self, urlopen):
        key = "sk-test-secret-123456789"
        body = io.BytesIO(f'{{"error":"echoed {key}"}}'.encode())
        urlopen.side_effect = urllib.error.HTTPError(
            "https://api.deepseek.com/chat/completions", 401, "unauthorized", {}, body
        )
        with mock.patch.dict(os.environ, {"DEEPSEEK_API_KEY": key}, clear=True):
            evidence = verifier.verify_deepseek("deepseek-chat")

        encoded = str(evidence)
        self.assertEqual("failed", evidence["outcome"])
        self.assertEqual(401, evidence["status_code"])
        self.assertNotIn(key, encoded)
        self.assertNotIn("echoed", encoded)

    @mock.patch.object(verifier.urllib.request, "urlopen", side_effect=RuntimeError("secret in transport"))
    def test_transport_exception_does_not_emit_exception_text(self, _urlopen):
        with mock.patch.dict(os.environ, {"DEEPSEEK_API_KEY": "sk-test-secret-123456789"}, clear=True):
            evidence = verifier.verify_deepseek("deepseek-chat")
        self.assertEqual("RuntimeError: provider request failed", evidence["error"])
        self.assertNotIn("secret in transport", str(evidence))

    def test_missing_credential_has_distinct_blocked_exit(self):
        with mock.patch.dict(os.environ, {}, clear=True), mock.patch.object(
            verifier.sys, "argv", ["verify_real_provider.py", "--provider", "deepseek"]
        ), mock.patch("builtins.print"):
            self.assertEqual(2, verifier.main())

    @mock.patch.object(verifier.urllib.request, "urlopen")
    def test_response_body_is_bounded(self, urlopen):
        class Response:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, *_args):
                return False

            def read(self, limit):
                self.limit = limit
                return b"x" * limit

        response = Response()
        urlopen.return_value = response
        with mock.patch.dict(os.environ, {"DEEPSEEK_API_KEY": "sk-test-secret-123456789"}, clear=True):
            evidence = verifier.verify_deepseek("deepseek-chat")
        self.assertEqual(verifier.MAX_RESPONSE_BYTES + 1, response.limit)
        self.assertEqual("failed", evidence["outcome"])
        self.assertIn("exceeded", evidence["error"])


if __name__ == "__main__":
    unittest.main()
