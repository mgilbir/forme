# Draws glyphs — the outlines themselves, not their boxes — with two other
# readers of the font program and writes what they drew, so that
# shape/outline_test.go can hold Face.GlyphOutline to them.
#
#   make hboutline
#
# The same bargain as cffink.py: the output is checked in, so running the test
# needs a Go toolchain and regenerating it needs Python and the pinned
# uharfbuzz and fontTools.
#
# # Two oracles
#
# HarfBuzz is what a browser draws with, and the one the rest of this package
# is held to. It draws every kind of outline the package reads: glyf with its
# composites resolved, CFF and CFF2 charstrings, VARC's composites, and any of
# them at a point of a design space (hb-draw). Its answers are the "hb" lines.
#
# fontTools is a second, independent parse of the same bytes: a glyf glyph's
# flattened coordinates (which resolves components, their transforms and the
# ones placed by matching points) and a charstring drawn through its pen. It
# cannot draw VARC, and its instances are not HarfBuzz's to within a unit, so
# it is asked only at the default instance. Its answers are the "ft" lines.
#
# # What is written
#
# A glyph's outline is a list of contours, each a start point and the edges
# that go on from it — "L" x y, "Q" cx cy x y, "C" c1x c1y c2x c2y x y — with
# the line back to the start made explicit where the contour does not end
# there. A contour's start is not compared: each reader begins a contour at a
# different one of its points, and the test matches the edges as a cycle.
#
# The faces are named on the command line as NAME=PATH, the location as
# NAME=PATH@{"wght":700} and a subset of the glyphs as NAME=PATH#STRIDE, which
# is every STRIDE-th glyph and a dozen composites. A face
# that is not there stops the run rather than being left out.
import hashlib
import json
import re
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
ft_version = fonttools().version
from fontTools.pens.basePen import BasePen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402
import io  # noqa: E402

out_path = sys.argv[1]
cases = []
for a in sys.argv[2:]:
    name, rest = a.split("=", 1)
    loc = {}
    stride = 1
    if "#" in rest:
        rest, s = rest.rsplit("#", 1)
        stride = int(s)
    if "@" in rest:
        rest, l = rest.split("@", 1)
        loc = json.loads(l)
    cases.append((name, rest, loc, stride))


def num(v):
    v = round(float(v), 4)
    return int(v) if v == int(v) else v


class Contours(BasePen):
    """Collects what a pen draws as contours, with the closing line explicit."""

    def __init__(self, glyphSet=None):
        super().__init__(glyphSet)
        self.contours = []
        self.cur = None

    def _moveTo(self, pt):
        self.cur = [pt, []]

    def _lineTo(self, pt):
        self.cur[1].append(("L", pt))

    def _curveToOne(self, p1, p2, p3):
        self.cur[1].append(("C", p1, p2, p3))

    def _qCurveToOne(self, p1, p2):
        self.cur[1].append(("Q", p1, p2))

    def _closePath(self):
        self.finish()

    def _endPath(self):
        self.finish()

    def finish(self):
        if self.cur is None:
            return
        start, edges = self.cur
        self.cur = None
        if not edges:
            return
        end = edges[-1][-1]
        if (round(end[0], 4), round(end[1], 4)) != (round(start[0], 4), round(start[1], 4)):
            edges.append(("L", start))
        self.contours.append((start, edges))


def encode(contours):
    return [[[num(s[0]), num(s[1])],
             [[e[0]] + [num(v) for p in e[1:] for v in p] for e in edges]]
            for s, edges in contours]


def glyf_contours(glyf, name):
    """One glyf glyph's contours from fontTools' flattened coordinates, in the
    usual way: off-curve points are the control points of quadratic curves whose
    on-curve points lie between them."""
    g = glyf[name]
    coords, ends, flags = g.getCoordinates(glyf)
    out = []
    first = 0
    for e in ends:
        pts = [tuple(c) for c in coords[first:e + 1]]
        on = [bool(f & 1) for f in flags[first:e + 1]]
        first = e + 1
        n = len(pts)
        if n < 2:
            continue
        mid = lambda a, b: ((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)
        if on[0]:
            start, seq = pts[0], list(zip(pts[1:], on[1:]))
        elif on[-1]:
            start, seq = pts[-1], list(zip(pts[:-1], on[:-1]))
        else:
            start, seq = mid(pts[0], pts[-1]), list(zip(pts, on))
        edges, ctrl = [], None
        for p, o in seq:
            if o and ctrl is not None:
                edges.append(("Q", ctrl, p))
                ctrl = None
            elif o:
                edges.append(("L", p))
            elif ctrl is not None:
                edges.append(("Q", ctrl, mid(ctrl, p)))
                ctrl = p
            else:
                ctrl = p
        if ctrl is not None:
            edges.append(("Q", ctrl, start))
        if edges:
            end = edges[-1][-1]
            if (round(end[0], 4), round(end[1], 4)) != (round(start[0], 4), round(start[1], 4)):
                edges.append(("L", start))
            out.append((start, edges))
    return out


lines = []
for name, path, loc, stride in cases:
    try:
        data = open(path, "rb").read()
    except FileNotFoundError:
        sys.exit(f"{name}: {path} is not there. Fetch the corpora it is in "
                 "(make noto-fonts cff-fonts) and run again.")
    face = hb.Face(data)
    font = hb.Font(face)
    if loc:
        font.set_variations(loc)
    n = face.glyph_count
    gids = sorted(set(range(0, n, stride)) | {n - 1}) if stride > 1 else list(range(n))
    if stride > 1:
        # A sample that missed every composite would say nothing about them:
        # a dozen of the font's, spread across it, are always in.
        tt = TTFont(io.BytesIO(data))
        if "glyf" in tt:
            order = tt.getGlyphOrder()
            comps = [g for g in range(n) if tt["glyf"][order[g]].isComposite()]
            gids = sorted(set(gids) | set(comps[::max(1, len(comps) // 12)]))
    lines.append(f"case {name} {path} {hashlib.sha256(data).hexdigest()} {json.dumps(loc, separators=(',', ':'))}")
    for gid in gids:
        pen = Contours()
        font.draw_glyph_with_pen(gid, pen)
        pen.finish()
        lines.append(f"hb {gid} {json.dumps(encode(pen.contours), separators=(',', ':'))}")
    # fontTools at the default instance only, and not for a font it cannot
    # draw: one with VARC. Nor for CFFInk, whose charstrings are built to say
    # what HarfBuzz's interpreter reads a particular way and fontTools' another
    # (cffink_fixture.py): it is HarfBuzz's reading this package is held to.
    tt = TTFont(io.BytesIO(data))
    if loc or "VARC" in tt or name == "CFFInk":
        continue
    order = tt.getGlyphOrder()
    gs = tt.getGlyphSet()
    for gid in gids:
        if "glyf" in tt:
            g = tt["glyf"][order[gid]]
            if hasattr(g, "xMin") and tt["hmtx"][order[gid]][1] != g.xMin:
                # A glyph whose side bearing is not where its box begins is
                # drawn by HarfBuzz, and by FreeType, moved so that it is.
                # fontTools' coordinates are not moved, so it is not drawing
                # the same glyph.
                lines.append(f"ft {gid} none")
                continue
            try:
                contours = glyf_contours(tt["glyf"], order[gid])
            except Exception:
                # A component matched on a phantom point, which fontTools'
                # flattened coordinates do not hold. HarfBuzz's answer stands
                # alone.
                lines.append(f"ft {gid} none")
                continue
        else:
            pen = Contours(gs)
            try:
                gs[order[gid]].draw(pen)
            except Exception:
                # A charstring fontTools cannot run: CFFInk.otf has glyphs
                # built to fail. What it drew before it failed is not an
                # outline; the Go test holds its reader to the ink HarfBuzz
                # states for the glyph instead.
                lines.append(f"ft {gid} none")
                continue
            pen.finish()
            contours = pen.contours
        lines.append(f"ft {gid} {json.dumps(encode(contours), separators=(',', ':'))}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/outline.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each case, a face, the location it was drawn at, and each glyph's\n")
    w.write("# outline as HarfBuzz (hb) and fontTools (ft) draw it: contours, each a\n")
    w.write("# start point and its edges, L x y, Q cx cy x y and C c1x c1y c2x c2y x y.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# fonttools {ft_version}\n")
    for line in lines:
        w.write(line + "\n")
