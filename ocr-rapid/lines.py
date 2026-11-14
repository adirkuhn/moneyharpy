"""Builds lines of text from the boxes RapidOCR detects.

RapidOCR finds each run of text on its own: on a till receipt "TOTAAL" at
the left and "25,92" at the right are two boxes. A reader of the text needs
them on one line, as printed. The boxes are put on lines by how much they
overlap vertically, after undoing the skew of the photo, and each line is
read left to right. Standard library only, so it is tested without the
models.
"""

import math
import statistics
from dataclasses import dataclass, field

# A box joins a line when it overlaps the line vertically by at least this
# part of the lower of the two.
MIN_OVERLAP = 0.5

# The skew of a photo is measured on boxes at least this many times as wide
# as high: the angle of a short box says little.
MIN_ASPECT_FOR_SKEW = 2.0

# A larger skew is not trusted; the boxes are then taken as they are.
MAX_SKEW = math.radians(20)


@dataclass
class Word:
    """A run of text RapidOCR read: its box (four corners clockwise from the
    top left, in pixels), text and confidence (0 to 1)."""

    box: list
    text: str
    score: float
    # The box in deskewed coordinates, set by group.
    left: float = 0.0
    right: float = 0.0
    top: float = 0.0
    bottom: float = 0.0


@dataclass
class Line:
    """Words on one line, left to right."""

    words: list = field(default_factory=list)
    top: float = 0.0
    bottom: float = 0.0

    @property
    def text(self):
        return " ".join(w.text for w in self.words)

    @property
    def box(self):
        """The line's bounding box in the image: [x0, y0, x1, y1]."""
        xs = [p[0] for w in self.words for p in w.box]
        ys = [p[1] for w in self.words for p in w.box]
        return [round(min(xs)), round(min(ys)), round(max(xs)), round(max(ys))]

    @property
    def score(self):
        """The mean confidence of the words."""
        return sum(w.score for w in self.words) / len(self.words)

    def add(self, word):
        self.words.append(word)
        # The line's band is the mean of its words', so one tall word does
        # not let it swallow the next line.
        self.top = statistics.fmean(w.top for w in self.words)
        self.bottom = statistics.fmean(w.bottom for w in self.words)

    def overlap(self, word):
        """How much word overlaps the line vertically, as a part of the lower
        of the two."""
        shared = min(self.bottom, word.bottom) - max(self.top, word.top)
        height = min(self.bottom - self.top, word.bottom - word.top)
        if height <= 0:
            return 1.0 if self.top <= word.top <= self.bottom else 0.0
        return shared / height

    def collides(self, word):
        """Whether word overlaps a word of the line horizontally: then it is
        on another line, such as small print under a large word."""
        for w in self.words:
            shared = min(w.right, word.right) - max(w.left, word.left)
            if shared > 0.3 * min(w.right - w.left, word.right - word.left):
                return True
        return False


def skew(words):
    """The angle in radians the text runs at: the median of the top edges of
    the wide boxes, or 0."""
    angles = []
    for w in words:
        (x0, y0), (x1, y1), _, (x3, y3) = w.box[:4]
        width, height = math.hypot(x1 - x0, y1 - y0), math.hypot(x3 - x0, y3 - y0)
        if width > 0 and width >= MIN_ASPECT_FOR_SKEW * height:
            angles.append(math.atan2(y1 - y0, x1 - x0))
    if not angles:
        return 0.0
    angle = statistics.median(angles)
    return angle if abs(angle) <= MAX_SKEW else 0.0


def group(words):
    """Puts the words on lines, top to bottom, each read left to right."""
    if not words:
        return []
    angle = skew(words)
    cos, sin = math.cos(angle), math.sin(angle)
    for w in words:
        # Rotate by -angle, so the text runs horizontally.
        xs = [x * cos + y * sin for x, y in w.box]
        ys = [y * cos - x * sin for x, y in w.box]
        w.left, w.right, w.top, w.bottom = min(xs), max(xs), min(ys), max(ys)
    lines = []
    for w in sorted(words, key=lambda w: (w.top + w.bottom) / 2):
        best, best_overlap = None, MIN_OVERLAP
        for line in lines:
            overlap = line.overlap(w)
            if overlap >= best_overlap and not line.collides(w):
                best, best_overlap = line, overlap
        if best is None:
            best = Line()
            lines.append(best)
        best.add(w)
    for line in lines:
        line.words.sort(key=lambda w: w.left)
    lines.sort(key=lambda line: (line.top + line.bottom) / 2)
    return lines


def text(lines):
    """The text of the lines, one per line."""
    return "\n".join(line.text for line in lines)
