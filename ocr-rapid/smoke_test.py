"""Smoke test with the real models on a synthetic receipt photo (receipt.py).
It needs the models, so it runs in the image's test stage
(Dockerfile.ocr-rapid), not with the unit tests:

    docker build -f Dockerfile.ocr-rapid --target test .
"""

import time
import unittest

from engine import Reader
from lines import group, text
from receipt import receipt
from server import Unreadable


class SmokeTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reader = Reader(threads=2)

    def test_reads_a_receipt_photo_with_its_columns_on_one_line(self):
        start = time.monotonic()
        lines = text(group(self.reader(receipt())))
        print(f"\nread the receipt in {time.monotonic() - start:.2f}s:\n{lines}")
        for want in [
            "Example Shop B.V.",
            "KVK 12345678",
            "NL123456782B01",
            "20-10-2026",
            "Crème brûlée 7,50",
            "TOTAAL 24,10",
            "BTW 9% over 22,11 1,99",
            "Pin 24,10",
        ]:
            self.assertIn(want, lines)

    def test_a_photo_without_text(self):
        from io import BytesIO

        from PIL import Image

        out = BytesIO()
        Image.new("RGB", (64, 64), "white").save(out, "PNG")
        self.assertEqual(self.reader(out.getvalue()), [])

    def test_not_an_image(self):
        with self.assertRaises(Unreadable):
            self.reader(b"%PDF-1.7 not an image")


if __name__ == "__main__":
    unittest.main()
