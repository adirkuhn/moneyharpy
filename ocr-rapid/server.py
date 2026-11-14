"""MoneyHarpy's RapidOCR service: reads the text of an image with RapidOCR
(PaddleOCR's PP-OCR models on ONNX Runtime) for the OCR service (cmd/ocrd),
which asks it next to Tesseract.

    POST /read   image bytes -> {"text": ..., "lines": [{"text", "box", "score"}]}
    GET  /healthz

It runs in its own container on an internal network, without internet
access: the models are in the image, and documents never leave the server.
It keeps nothing and logs no text.

Settings, from the environment:

    LISTEN_ADDR          default :8091
    MAX_BODY_MB          largest image, default 20
    RAPIDOCR_WORKERS     images read at once, default 1
    RAPIDOCR_THREADS     ONNX Runtime threads per worker, default the CPUs
                         (within the container's CPU quota) shared by the
                         workers, at most 4: more threads made a receipt
                         slower, not faster
    QUEUE_TIMEOUT_SECONDS  how long a request waits for a free worker, 120
"""

import json
import os
import queue
import sys
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from lines import group, text


# The default most ONNX Runtime threads per worker.
MAX_THREADS = 4


class Unreadable(Exception):
    """The body is not an image that can be read."""


def log(level, msg, **fields):
    """Writes a JSON log line, like the Go services."""
    print(json.dumps({"time": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "level": level, "msg": msg, **fields}), flush=True)


class Handler(BaseHTTPRequestHandler):
    server_version = "rapidocrd"
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):  # noqa: A002 - the base class's name
        pass  # Requests are logged in reply, without the client's details.

    def reply(self, status, body, content_type="text/plain; charset=utf-8"):
        if isinstance(body, str):
            body = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        if status >= 400:
            self.send_header("Connection", "close")
            self.close_connection = True
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self.reply(200, "ok\n")
        elif self.path == "/read":
            self.reply(405, "method not allowed\n")
        else:
            self.reply(404, "not found\n")

    def do_POST(self):
        if self.path != "/read":
            self.reply(404, "not found\n")
            return
        start = time.monotonic()
        status = self.read()
        log("INFO", "read", status=status, duration_ms=round((time.monotonic() - start) * 1000))

    def read(self):
        srv = self.server
        try:
            length = int(self.headers.get("Content-Length", ""))
        except ValueError:
            self.reply(411, "Content-Length is required\n")
            return 411
        if length > srv.max_body:
            self.reply(413, "image too large\n")
            return 413
        if length <= 0:
            self.reply(400, "no image\n")
            return 400
        data = self.rfile.read(length)
        try:
            reader = srv.readers.get(timeout=srv.wait)
        except queue.Empty:
            self.reply(503, "busy\n")
            return 503
        try:
            words = reader(data)
        except Unreadable as e:
            log("INFO", "unreadable image", error=str(e))
            self.reply(422, "the image cannot be read\n")
            return 422
        except Exception as e:  # noqa: BLE001 - any failure of the models is a 500
            log("ERROR", "read failed", error=repr(e))
            self.reply(500, "internal error\n")
            return 500
        finally:
            srv.readers.put(reader)
        lines = group(words)
        out = {
            "text": text(lines),
            "lines": [{"text": line.text, "box": line.box, "score": round(line.score, 3)} for line in lines],
        }
        self.reply(200, json.dumps(out, ensure_ascii=False), "application/json")
        return 200


def make_server(addr, readers, max_body=20 << 20, wait=120.0):
    """An HTTP server reading images with readers, functions from image
    bytes to the Words read; each reads one image at a time."""
    srv = ThreadingHTTPServer(addr, Handler)
    srv.daemon_threads = True
    srv.readers = queue.Queue()
    for r in readers:
        srv.readers.put(r)
    srv.max_body = max_body
    srv.wait = wait
    return srv


def cpus():
    """The CPUs this process may use: its affinity, capped by the cgroup's
    CPU quota (docker run --cpus)."""
    n = len(os.sched_getaffinity(0)) if hasattr(os, "sched_getaffinity") else (os.cpu_count() or 1)
    try:
        with open("/sys/fs/cgroup/cpu.max") as f:
            quota, period = f.read().split()
        if quota != "max":
            n = min(n, max(1, round(int(quota) / int(period))))
    except (OSError, ValueError):
        pass
    return max(n, 1)


def env_int(name, default):
    try:
        n = int(os.environ.get(name, ""))
    except ValueError:
        return default
    return n if n > 0 else default


def split_addr(addr):
    host, _, port = addr.rpartition(":")
    return host or "0.0.0.0", int(port)


def healthcheck(addr):
    host, port = split_addr(addr)
    if host == "0.0.0.0":
        host = "127.0.0.1"
    with urllib.request.urlopen(f"http://{host}:{port}/healthz", timeout=3) as res:  # noqa: S310 - local only
        return res.status == 200


def main():
    addr = os.environ.get("LISTEN_ADDR", ":8091")
    if sys.argv[1:] == ["-healthcheck"]:
        try:
            sys.exit(0 if healthcheck(addr) else 1)
        except OSError as e:
            print(e, file=sys.stderr)
            sys.exit(1)
    from engine import Reader  # The models load only when serving.

    workers = env_int("RAPIDOCR_WORKERS", 1)
    threads = env_int("RAPIDOCR_THREADS", max(1, min(MAX_THREADS, cpus() // workers)))
    readers = [Reader(threads) for _ in range(workers)]
    srv = make_server(
        split_addr(addr),
        readers,
        max_body=env_int("MAX_BODY_MB", 20) << 20,
        wait=float(env_int("QUEUE_TIMEOUT_SECONDS", 120)),
    )
    log("INFO", "listening", addr=addr, workers=workers, threads=threads, models=readers[0].models)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass

