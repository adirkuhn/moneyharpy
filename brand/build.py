"""Builds the web logos from the traced originals in brand/.

Run: python3 brand/build.py brand web/public/brand
"""
import re, sys
src, out = sys.argv[1], sys.argv[2]
def paths(svg): return re.findall(r'<path fill="(#[0-9a-f]{6})" d="([^"]*)"/>', svg)
def lum(c): r,g,b=(int(c[i:i+2],16) for i in (1,3,5)); return (0.299*r+0.587*g+0.114*b)
def start(d):
    m = re.match(r'M\s*([-\d.]+)[ ,]([-\d.]+)', d); return float(m[1]), float(m[2])
def write(name, viewbox, items):
    body = ''.join(f'<path fill="{f}" d="{d}"/>' for f, d in items)
    open(f'{out}/{name}', 'w').write(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{viewbox}">{body}</svg>\n')

head = paths(open(f'{src}/harpy.svg').read())
head = [p for p in head if not p[1].startswith('M0 0l600 0')]
write('harpy-mark.svg', '70 100 470 470', head)

word = paths(open(f'{src}/harpy-2.svg').read())
word = [p for p in word if not p[1].startswith('M0 0l660 0')]
write('harpy-logo.svg', '0 0 660 160', word)
# Dark variant: the navy letters of "Money" (right of the bird) become white.
# Dark variant: the navy letters of "Money" become white, the light tracing
# halos around the letters go, and the bird's navy lightens to stay visible.
def on_dark(f, d):
    x = start(d)[0]
    if x > 170:
        if lum(f) > 240:
            return None
        # Letters become one flat colour per word; tracing shades break up
        # at small sizes on a dark background.
        return '#ffffff' if x < 425 else '#13bdb5'
    return '#3f6db3' if lum(f) < 60 else f
dark = [(c, d) for f, d in word if (c := on_dark(f, d))]
write('harpy-logo-dark.svg', '0 0 660 160', dark)
mark_dark = [('#3f6db3' if lum(f) < 60 else f, d) for f, d in head]
write('harpy-mark-dark.svg', '70 100 470 470', mark_dark)
print(len(head), len(word), sum(1 for a, b in zip(word, dark) if a != b))
