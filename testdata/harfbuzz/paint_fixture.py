# Builds ColourPaint.ttf, the colour face paint.py asks HarfBuzz to paint, into
# the directory it is given:
#
#   make hbpaint
#
# ColourInk.ttf (colrink_fixture.py) paints with every paint format and every
# thing painting can do to a box, but it was built to be measured, and what
# measuring reads of a fill is only where it is: its one palette has two
# colours, its gradients one colour line, and none of its colour stops varies.
# Painting hands out the fill itself, so this face states what a fill can be:
#
#   - two palettes of three colours, some of them partly transparent, and
#     colour indices past the palette's end and the foreground's, 0xFFFF;
#   - solid fills at an alpha, fixed and variable;
#   - linear, radial and sweep gradients, each under the three extend modes,
#     with stops out of order, a stop in the foreground colour, a colour line
#     with no stops at all and a radius only an unsigned number holds;
#   - the variable gradients, whose points, radii, angles, stop offsets and
#     stop alphas move with the weight axis;
#   - fills inside transforms, layers, a composite and a colour glyph painting
#     another;
#   - COLRv0 layers in a palette colour, the foreground and one past the end;
#   - and a glyph with no colour at all, which is painted as its outline in the
#     foreground.
#
# SVGPaint.ttf is BitmapInk.ttf (colrink_fixture.py, built by hbcolrink first)
# with an SVG table and a COLRv0 glyph added: SVG documents, which are painted
# after COLR and before the bitmaps — a gzipped one, one drawing two glyphs,
# one whose stated length runs past the end of the table, and one that starts
# past it. Glyph 1 has a document and a bitmap, and is painted from the
# document; glyph 2 has a document and a COLR glyph, and is painted from COLR;
# glyph 4 has no document, and is painted from its bitmap; glyph 8's document
# starts past the end of the table, so it has none, and is painted as its
# outline.
#
# They are built with fontTools and their timestamps fixed, so that building
# them again produces the same bytes and the checksum the expectations record
# stays true.
import gzip
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.colorLib.builder import buildCOLR, buildCPAL  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402
from fontTools.ttLib.tables.DefaultTable import DefaultTable  # noqa: E402
from fontTools.ttLib.tables import otTables as ot  # noqa: E402
from fontTools.varLib.builder import buildDeltaSetIndexMap  # noqa: E402
from fontTools.varLib.varStore import OnlineVarStoreBuilder  # noqa: E402

F = ot.PaintFormat
FOREGROUND = 0xFFFF


def poly(*pts):
    pen = TTGlyphPen(None)
    pen.moveTo(pts[0])
    for p in pts[1:]:
        pen.lineTo(p)
    pen.closePath()
    return pen.glyph()


# The weight axis runs from 100 to 900 about 400. var records a field's deltas
# at the heaviest end, in the field's own units, and half of each the other way
# at the lightest, and names where they start: the VarIndexBase.
DELTAS = []


def var(*deltas):
    base = len(DELTAS)
    DELTAS.extend(deltas)
    return base


def var_store():
    builder = OnlineVarStoreBuilder(["wght"])
    builder.setSupports([{"wght": (0, 1.0, 1.0)}, {"wght": (-1.0, -1.0, 0)}])
    indices = [builder.storeDeltas([d, -(d // 2)]) for d in DELTAS]
    return builder.finish(optimize=False), buildDeltaSetIndexMap(indices)


SQ, TRI = "sq", "tri"


def fill(outline, paint):
    return {"Format": F.PaintGlyph, "Glyph": outline, "Paint": paint}


def solid(index, alpha=1.0):
    return {"Format": F.PaintSolid, "PaletteIndex": index, "Alpha": alpha}


def stop(offset, index, alpha=1.0):
    return {"StopOffset": offset, "PaletteIndex": index, "Alpha": alpha}


def line(extend, *stops):
    return {"Extend": extend, "ColorStop": list(stops)}


STOPS = [stop(0.0, 0), stop(0.5, FOREGROUND, 0.5), stop(1.0, 2, 0.75)]
# Out of order, past both ends, and at one offset twice: the stops are handed
# out as the font states them.
UNSORTED = [stop(1.25, 1), stop(-0.25, 0, 0.3), stop(0.5, 2), stop(0.5, FOREGROUND)]


def linear(extend, stops=STOPS):
    return {"Format": F.PaintLinearGradient, "ColorLine": line(extend, *stops),
            "x0": 0, "y0": 0, "x1": 400, "y1": 100, "x2": -50, "y2": 400}


def radial(extend, stops=STOPS):
    return {"Format": F.PaintRadialGradient, "ColorLine": line(extend, *stops),
            "x0": 100, "y0": 120, "r0": 10, "x1": 220, "y1": 200, "r1": 300}


def sweep(extend, stops=STOPS):
    return {"Format": F.PaintSweepGradient, "ColorLine": line(extend, *stops),
            "centerX": 200, "centerY": 210, "startAngle": -0.5, "endAngle": 0.75}


def varstops():
    return [dict(stop(0.0, 0), VarIndexBase=var(2048, -4096)),
            dict(stop(0.6, FOREGROUND, 0.8), VarIndexBase=var(-1024, 3000)),
            dict(stop(1.0, 1, 0.5), VarIndexBase=var(0, 8192))]


COLOR = {
    # Solid fills.
    "p_solid": fill(SQ, solid(0)),
    "p_alpha": fill(SQ, solid(1, 0.5)),
    "p_foreground": fill(SQ, solid(FOREGROUND, 0.75)),
    "p_pastpalette": fill(SQ, solid(7)),
    "p_varsolid": fill(SQ, {"Format": F.PaintVarSolid, "PaletteIndex": 2, "Alpha": 0.6,
                            "VarIndexBase": var(-4096)}),
    # A solid fill not directly inside its outline, which is painted as a
    # fill rather than folded into the outline's.
    "p_solidinside": fill(SQ, {"Format": F.PaintTranslate, "Paint": solid(1), "dx": 10, "dy": 20}),
    # The gradients under each extend mode.
    "p_linear_pad": fill(SQ, linear("pad")),
    "p_linear_repeat": fill(SQ, linear("repeat")),
    "p_linear_reflect": fill(SQ, linear("reflect")),
    "p_radial_pad": fill(TRI, radial("pad")),
    "p_radial_repeat": fill(TRI, radial("repeat")),
    "p_radial_reflect": fill(TRI, radial("reflect")),
    "p_sweep_pad": fill(SQ, sweep("pad")),
    "p_sweep_repeat": fill(SQ, sweep("repeat")),
    "p_sweep_reflect": fill(SQ, sweep("reflect")),
    "p_unsorted": fill(SQ, linear("pad", UNSORTED)),
    # A radius past 32,767, which a radius, unsigned, can be and a coordinate
    # cannot.
    "p_wideradial": fill(TRI, dict(radial("pad"), r1=40000)),
    "p_nostops": fill(SQ, radial("pad", [])),
    # The variable gradients.
    "p_varlinear": fill(SQ, {
        "Format": F.PaintVarLinearGradient, "ColorLine": line("reflect", *varstops()),
        "x0": 0, "y0": 0, "x1": 400, "y1": 100, "x2": -50, "y2": 400,
        "VarIndexBase": var(30, -20, 60, 10, -45, 25)}),
    "p_varradial": fill(TRI, {
        "Format": F.PaintVarRadialGradient, "ColorLine": line("repeat", *varstops()),
        "x0": 100, "y0": 120, "r0": 10, "x1": 220, "y1": 200, "r1": 300,
        "VarIndexBase": var(-15, 40, 25, 10, -30, -120)}),
    "p_varsweep": fill(SQ, {
        "Format": F.PaintVarSweepGradient, "ColorLine": line("pad", *varstops()),
        "centerX": 200, "centerY": 210, "startAngle": -0.5, "endAngle": 0.75,
        "VarIndexBase": var(35, -25, 1024, -2048)}),
    # Fills inside what paints them.
    "p_transformed": {"Format": F.PaintTranslate, "dx": 40, "dy": -30, "Paint": {
        "Format": F.PaintRotate, "angle": 0.125, "Paint": fill(TRI, radial("pad"))}},
    "p_layers": {"Format": F.PaintColrLayers, "Layers": [
        fill(SQ, solid(2, 0.4)), fill(TRI, linear("repeat")), fill(SQ, sweep("reflect"))]},
    "p_composite": {"Format": F.PaintComposite, "CompositeMode": "MULTIPLY",
                    "SourcePaint": fill(TRI, solid(1)),
                    "BackdropPaint": fill(SQ, linear("pad"))},
    "p_hue": {"Format": F.PaintComposite, "CompositeMode": "HSL_LUMINOSITY",
              "SourcePaint": fill(TRI, solid(FOREGROUND)),
              "BackdropPaint": fill(SQ, solid(0))},
    "p_colrglyph": {"Format": F.PaintColrGlyph, "Glyph": "p_varlinear"},
}

# COLRv0 glyphs: each layer a solid fill of an outline.
V0 = {
    "v0_palette": [(SQ, 0), (TRI, 1)],
    "v0_foreground": [(SQ, 2), (TRI, FOREGROUND)],
    "v0_pastpalette": [(TRI, 9)],
}

PALETTES = [
    [(1.0, 0.0, 0.0, 1.0), (0.0, 0.0, 1.0, 0.5), (0.0, 0.6, 0.2, 0.25)],
    [(1.0, 0.8, 0.0, 1.0), (0.2, 0.2, 0.2, 0.75), (0.5, 0.0, 0.5, 1.0)],
]


def build(path):
    order = [".notdef", SQ, TRI, "plain"] + sorted(COLOR) + sorted(V0)
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41: "p_solid", 0x42: "plain"})
    glyf = {
        ".notdef": poly((50, 0), (50, 700), (450, 700), (450, 0)),
        SQ: poly((0, 0), (0, 400), (400, 400), (400, 0)),
        TRI: poly((0, -20), (250, 480), (500, -20)),
        "plain": poly((100, 0), (100, 500), (300, 500)),
    }
    for name in list(COLOR) + list(V0):
        glyf[name] = poly((10, 10), (10, 60), (60, 60), (60, 10))
    fb.setupGlyf(glyf)
    fb.setupHorizontalMetrics({name: (600, 0) for name in order})
    fb.setupHorizontalHeader(ascent=900, descent=-200)
    fb.setupOS2(sTypoAscender=900, sTypoDescender=-200, usWinAscent=900, usWinDescent=200)
    fb.setupNameTable({"familyName": "ColourPaint", "styleName": "Regular"})
    fb.setupPost()
    glyphs = dict(COLOR)
    glyphs.update(V0)
    colr = buildCOLR(glyphs, glyphMap=fb.font.getReverseGlyphMap())
    colr.table.VarStore, colr.table.VarIndexMap = var_store()
    fb.setupFvar([("wght", 100, 400, 900, "Weight")], [])
    fb.font["COLR"] = colr
    fb.font["CPAL"] = buildCPAL(PALETTES)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def svg_doc(*glyphs):
    shapes = "".join(
        f'<g id="glyph{g}"><rect x="{50 * g}" y="-900" width="400" height="{100 * g}" fill="#{g}{g}0000"/></g>'
        for g in glyphs)
    return ('<svg xmlns="http://www.w3.org/2000/svg" version="1.1">' + shapes + "</svg>").encode()


def svg_table():
    """An SVG table written byte by byte, so that a document can state a
    length or an offset past the end of the table: each index entry is a
    glyph range, an offset from the start of the index, and a length."""
    docs = [
        (1, 1, svg_doc(1)),
        # gzip with its timestamp fixed, so that the bytes do not move.
        (2, 3, gzip.compress(svg_doc(2, 3), mtime=0)),
        (5, 6, svg_doc(5, 6)),
        (7, 7, svg_doc(7)),
    ]
    index_size = 2 + 12 * (len(docs) + 1)
    entries, blobs, at = [], b"", index_size
    for first, last, data in docs:
        entries.append((first, last, at, len(data)))
        blobs += data
        at += len(data)
    # Glyph 7's document states a length past the end of the table, which
    # HarfBuzz cuts at the end; glyph 8's starts past it, which is none.
    first, last, off, n = entries[-1]
    entries[-1] = (first, last, off, n + 1000)
    entries.append((8, 8, at + 5000, 10))
    index = struct.pack(">H", len(entries)) + b"".join(struct.pack(">HHII", *e) for e in entries)
    return struct.pack(">HII", 0, 10, 0) + index + blobs


def build_svg(directory):
    font = TTFont(os.path.join(directory, "BitmapInk.ttf"))
    svg = DefaultTable("SVG ")
    svg.data = svg_table()
    font["SVG "] = svg
    font["COLR"] = buildCOLR({"bm2": [("bm1", 0)]})
    font["CPAL"] = buildCPAL([[(0.0, 0.5, 0.0, 1.0)]])
    font["name"].setName("SVGPaint", 1, 3, 1, 0x409)
    font["head"].created = font["head"].modified = 3660681600
    font.recalcTimestamp = False
    font.save(os.path.join(directory, "SVGPaint.ttf"))


if __name__ == "__main__":
    build(os.path.join(sys.argv[1], "ColourPaint.ttf"))
    build_svg(sys.argv[1])
