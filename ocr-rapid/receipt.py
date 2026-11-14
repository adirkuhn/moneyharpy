"""A synthetic phone photo of a till receipt, for the smoke test and to
measure the service.

Everything on it is made up: Example Shop B.V., KVK 12345678 and VAT number
NL123456782B01 are the project's example data.

    python3 receipt.py > receipt.jpg
"""

import io
import sys

from PIL import Image, ImageDraw, ImageFilter, ImageFont

# The receipt as printed: text on the left, an amount on the right.
LINES = [
    ("Example Shop B.V.", ""),
    ("Voorbeeldstraat 1, 1234 AB Utrecht", ""),
    ("KVK 12345678", ""),
    ("BTW NL123456782B01", ""),
    ("", ""),
    ("Datum 20-10-2026 14:32", ""),
    ("", ""),
    ("2 x Koffie", "6,40"),
    ("Appeltaart", "4,25"),
    ("Broodje kaas", "5,95"),
    ("Crème brûlée", "7,50"),
    ("", ""),
    ("TOTAAL", "24,10"),
    ("", ""),
    ("BTW 9% over 22,11", "1,99"),
    ("Pin", "24,10"),
    ("", ""),
    ("Bedankt en tot ziens!", ""),
]

# Fonts tried in turn; a monospaced one looks like a till printer.
FONTS = [
    "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
]


def _font(size):
    for path in FONTS:
        try:
            return ImageFont.truetype(path, size)
        except OSError:
            continue
    return ImageFont.load_default(size)


def receipt(angle=1.5):
    """Returns the JPEG of a phone photo of the receipt: paper on a dark
    table, a little rotated, soft, noisy and unevenly lit."""
    font = _font(30)
    width, line_height, margin = 760, 46, 40
    paper = Image.new("L", (width, margin * 2 + line_height * len(LINES)), 236)
    draw = ImageDraw.Draw(paper)
    for i, (left, right) in enumerate(LINES):
        y = margin + i * line_height
        if left:
            draw.text((margin, y), left, font=font, fill=30)
        if right:
            draw.text((width - margin - draw.textlength(right, font=font), y), right, font=font, fill=30)
    photo = Image.new("L", (paper.width + 300, paper.height + 300), 70)
    photo.paste(paper, (150, 150))
    photo = photo.rotate(angle, resample=Image.BICUBIC, fillcolor=70)
    photo = photo.filter(ImageFilter.GaussianBlur(1.1))
    photo = Image.blend(photo, Image.effect_noise(photo.size, 40), 0.08)
    photo = photo.resize((photo.width * 3 // 2, photo.height * 3 // 2), Image.BICUBIC)
    light = Image.linear_gradient("L").resize(photo.size).point(lambda v: 255 - v // 5)
    photo = Image.composite(photo, Image.new("L", photo.size, 0), light)
    out = io.BytesIO()
    photo.convert("RGB").save(out, "JPEG", quality=85)
    return out.getvalue()


if __name__ == "__main__":
    sys.stdout.buffer.write(receipt())
