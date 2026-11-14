"""Tests of the HTTP service with a fake reader instead of the models.
Standard library only."""

import http.client
import json
import threading
import time
import unittest
from unittest import mock

from lines import Word
from server import Unreadable, make_server


def fake_read(data):
    if data == b"broken":
        raise Unreadable("not an image")
    if data == b"crash":
        raise RuntimeError("onnxruntime failed")
    if data == b"slow":
        time.sleep(0.3)
    return [
        Word([[600, 102], [700, 102], [700, 138], [600, 138]], "25,92", 0.99),
        Word([[40, 100], [200, 100], [200, 140], [40, 140]], "TOTAAL", 0.97),
    ]


class ServerTest(unittest.TestCase):
    def setUp(self):
        patch = mock.patch("server.log")
        patch.start()
        self.addCleanup(patch.stop)

    def start(self, readers=None, max_body=1024, wait=5.0):
        srv = make_server(("127.0.0.1", 0), readers or [fake_read], max_body=max_body, wait=wait)
        threading.Thread(target=srv.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True).start()
        self.addCleanup(srv.server_close)
        self.addCleanup(srv.shutdown)
        return srv.server_address[1]

    def request(self, port, method, path, body=None):
        conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
        self.addCleanup(conn.close)
        conn.request(method, path, body=body)
        res = conn.getresponse()
        return res.status, res.read()

    def test_read_returns_text_and_lines(self):
        port = self.start()
        status, body = self.request(port, "POST", "/read", b"\xff\xd8\xff image")
        self.assertEqual(status, 200)
        out = json.loads(body)
        self.assertEqual(out["text"], "TOTAAL 25,92")
        self.assertEqual(out["lines"], [{"text": "TOTAAL 25,92", "box": [40, 100, 700, 140], "score": 0.98}])

    def test_healthz(self):
        port = self.start()
        self.assertEqual(self.request(port, "GET", "/healthz"), (200, b"ok\n"))

    def test_unknown_paths(self):
        port = self.start()
        self.assertEqual(self.request(port, "GET", "/read")[0], 405)
        self.assertEqual(self.request(port, "POST", "/other", b"x")[0], 404)

    def test_errors(self):
        port = self.start()
        self.assertEqual(self.request(port, "POST", "/read", b"broken")[0], 422)
        self.assertEqual(self.request(port, "POST", "/read", b"crash")[0], 500)
        self.assertEqual(self.request(port, "POST", "/read", b"")[0], 400)
        self.assertEqual(self.request(port, "POST", "/read", b"x" * 2048)[0], 413)

    def test_waits_for_a_free_reader_and_gives_up(self):
        port = self.start(wait=0.05)
        results = []
        slow = threading.Thread(target=lambda: results.append(self.request(port, "POST", "/read", b"slow")[0]))
        slow.start()
        time.sleep(0.1)
        self.assertEqual(self.request(port, "POST", "/read", b"image")[0], 503)
        slow.join()
        self.assertEqual(results, [200])

    def test_readers_work_at_once(self):
        port = self.start(readers=[fake_read, fake_read])
        results = []
        threads = [
            threading.Thread(target=lambda: results.append(self.request(port, "POST", "/read", b"slow")[0]))
            for _ in range(2)
        ]
        start = time.monotonic()
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.assertEqual(results, [200, 200])
        self.assertLess(time.monotonic() - start, 0.55)


if __name__ == "__main__":
    unittest.main()
