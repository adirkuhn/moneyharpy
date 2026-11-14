"""Tests of the line building, with synthetic boxes. Standard library only:

    python3 -m unittest discover -s ocr-rapid
"""

import math
import unittest

from lines import Word, group, text


def box(x0, y0, x1, y1, angle=0.0):
    """A detected box, corners clockwise from the top left, rotated by angle
    degrees about the origin as a skewed photo would be."""
    a = math.radians(angle)
    return [
        [x * math.cos(a) - y * math.sin(a), x * math.sin(a) + y * math.cos(a)]
        for x, y in ((x0, y0), (x1, y0), (x1, y1), (x0, y1))
    ]


class GroupTest(unittest.TestCase):
    def test_nothing(self):
        self.assertEqual(group([]), [])
        self.assertEqual(text([]), "")

    def test_columns_of_a_receipt_stay_on_one_line(self):
        words = [
            Word(box(600, 102, 700, 138), "25,92", 0.99),
            Word(box(40, 100, 200, 140), "TOTAAL", 0.98),
            Word(box(40, 40, 400, 80), "Example Shop B.V.", 0.97),
            Word(box(40, 160, 180, 200), "BTW 21%", 0.9),
            Word(box(610, 162, 700, 198), "4,50", 0.8),
        ]
        lines = group(words)
        self.assertEqual(
            [line.text for line in lines],
            ["Example Shop B.V.", "TOTAAL 25,92", "BTW 21% 4,50"],
        )
        self.assertEqual(lines[1].box, [40, 100, 700, 140])
        self.assertAlmostEqual(lines[2].score, 0.85)

    def test_words_read_left_to_right(self):
        words = [
            Word(box(300, 10, 400, 40), "ziens!", 1),
            Word(box(10, 10, 100, 40), "Tot", 1),
            Word(box(150, 12, 250, 42), "en", 1),
        ]
        self.assertEqual(text(group(words)), "Tot en ziens!")

    def test_a_skewed_photo_keeps_its_rows(self):
        # Rotated 3 degrees, the amount at the far right of a row sits
        # higher in the image than the next row's text at the left.
        rows = [("Koffie", "3,20"), ("Appeltaart", "4,25"), ("TOTAAL", "7,45")]
        words = []
        for i, (left, right) in enumerate(rows):
            y = 100 + 45 * i
            words.append(Word(box(40, y, 300, y + 34, angle=-3), left, 1))
            words.append(Word(box(900, y, 1000, y + 34, angle=-3), right, 1))
        self.assertEqual(text(group(words)), "Koffie 3,20\nAppeltaart 4,25\nTOTAAL 7,45")

    def test_skew_the_other_way(self):
        words = [
            Word(box(40, 100, 300, 134, angle=4), "TOTAAL", 1),
            Word(box(900, 100, 1000, 134, angle=4), "7,45", 1),
            Word(box(40, 145, 300, 179, angle=4), "Pin", 1),
            Word(box(900, 145, 1000, 179, angle=4), "7,45", 1),
        ]
        self.assertEqual(text(group(words)), "TOTAAL 7,45\nPin 7,45")

    def test_lines_that_barely_touch_stay_apart(self):
        words = [
            Word(box(40, 100, 300, 140), "Datum 20-10-2026", 1),
            Word(box(40, 132, 300, 172), "Kassa 3", 1),
        ]
        self.assertEqual(text(group(words)), "Datum 20-10-2026\nKassa 3")

    def test_stacked_words_in_one_band_are_separate_lines(self):
        # Small print under a large word overlaps it vertically but sits
        # below it at the same x: two lines, not one.
        words = [
            Word(box(40, 100, 400, 200), "EXAMPLE", 1),
            Word(box(60, 170, 300, 200), "Shop", 1),
        ]
        self.assertEqual(text(group(words)), "EXAMPLE\nShop")

    def test_a_tall_word_spanning_two_lines_joins_one(self):
        words = [
            Word(box(40, 100, 200, 130), "Klant", 1),
            Word(box(40, 135, 200, 165), "Kassa", 1),
            Word(box(400, 112, 500, 150), "12", 1),
        ]
        lines = group(words)
        self.assertEqual(len(lines), 2)
        self.assertEqual(sum(len(line.words) for line in lines), 3)

    def test_dutch_accents_pass_through(self):
        words = [Word(box(10, 10, 300, 40), "Crème brûlée", 0.95), Word(box(500, 10, 600, 40), "7,50", 1)]
        self.assertEqual(text(group(words)), "Crème brûlée 7,50")

    def test_degenerate_boxes(self):
        words = [Word([[5, 5], [5, 5], [5, 5], [5, 5]], ".", 0.1), Word(box(10, 0, 100, 10), "Pin", 1)]
        self.assertEqual(sorted(text(group(words)).split()), [".", "Pin"])


if __name__ == "__main__":
    unittest.main()
