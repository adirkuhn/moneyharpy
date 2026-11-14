"""RapidOCR with the models baked into the image.

Models (PaddleOCR PP-OCRv5, Apache-2.0, as ONNX from RapidOCR's model
repository; Dockerfile.ocr-rapid downloads them at build time and checks
their SHA-256):

    det  ch_PP-OCRv5_det_mobile        finds text in any script
    cls  ch_ppocr_mobile_v2.0_cls      turns text that is upside down
    rec  latin_PP-OCRv5_rec_mobile     reads Latin script, with accents
                                       (Dutch, German, French, ...)

The paths are given to RapidOCR, so it never downloads anything.

ONNX Runtime's Linux build reports telemetry to Microsoft unless
ORT_DISABLE_TELEMETRY is set; it is set before ONNX Runtime loads (and in
the image), so its telemetry is never started. The container has no
internet access either.
"""

import io
import os
import warnings

os.environ["ORT_DISABLE_TELEMETRY"] = "1"

import onnxruntime  # noqa: E402 - after switching telemetry off
from PIL import Image, ImageDraw, ImageFont, ImageOps
from rapidocr import RapidOCR
from rapidocr.utils.typings import EngineType, ModelType, OCRVersion

from lines import Word
from server import Unreadable

onnxruntime.disable_telemetry_events()

MODELS = os.environ.get("RAPIDOCR_MODELS", "/models")
DET = "ch_PP-OCRv5_det_mobile.onnx"
CLS = "ch_ppocr_mobile_v2.0_cls_mobile.onnx"
REC = "latin_PP-OCRv5_rec_mobile.onnx"

# The most pixels an image may have; a small file must not claim gigabytes.
MAX_PIXELS = 100_000_000
Image.MAX_IMAGE_PIXELS = MAX_PIXELS


class Reader:
    """Reads images with one RapidOCR engine; not for two images at once."""

    def __init__(self, threads):
        for name in (DET, CLS, REC):
            if not os.path.isfile(os.path.join(MODELS, name)):
                raise FileNotFoundError(f"model {name} is not in {MODELS}")
        self.models = {"det": DET, "cls": CLS, "rec": REC}
        self.engine = RapidOCR(
            params={
                "Global.log_level": "warning",
                "EngineConfig.onnxruntime.intra_op_num_threads": threads,
                "EngineConfig.onnxruntime.inter_op_num_threads": 1,
                "Det.engine_type": EngineType.ONNXRUNTIME,
                "Det.lang_type": "ch",
                "Det.model_type": ModelType.MOBILE,
                "Det.ocr_version": OCRVersion.PPOCRV5,
                "Det.model_path": os.path.join(MODELS, DET),
                "Cls.engine_type": EngineType.ONNXRUNTIME,
                "Cls.lang_type": "ch",
                "Cls.model_type": ModelType.MOBILE,
                "Cls.ocr_version": OCRVersion.PPOCRV4,
                "Cls.model_path": os.path.join(MODELS, CLS),
                "Rec.engine_type": EngineType.ONNXRUNTIME,
                "Rec.lang_type": "latin",
                "Rec.model_type": ModelType.MOBILE,
                "Rec.ocr_version": OCRVersion.PPOCRV5,
                "Rec.model_path": os.path.join(MODELS, REC),
            }
        )
        self.warm_up()

    def warm_up(self):
        """Reads a line of text once: ONNX Runtime's first run is several
        times slower than the next, and should not be a user's."""
        img = Image.new("RGB", (400, 60), "white")
        ImageDraw.Draw(img).text((10, 10), "Totaal 12,34", font=ImageFont.load_default(32), fill="black")
        self.engine(img)

    def __call__(self, data):
        try:
            with warnings.catch_warnings():
                warnings.simplefilter("error", Image.DecompressionBombWarning)
                img = Image.open(io.BytesIO(data))
                if img.width * img.height > MAX_PIXELS:
                    raise Unreadable(f"{img.width}x{img.height} pixels is too large")
                # A phone stores the photo as taken and says in EXIF how to
                # turn it.
                img = ImageOps.exif_transpose(img).convert("RGB")
        except (OSError, ValueError, SyntaxError, Image.DecompressionBombError, Image.DecompressionBombWarning) as e:
            raise Unreadable(str(e)) from e
        out = self.engine(img)
        if out.boxes is None or out.txts is None:
            return []
        return [
            Word([[float(x), float(y)] for x, y in box], str(txt), float(score))
            for box, txt, score in zip(out.boxes, out.txts, out.scores)
        ]
