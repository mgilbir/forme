# Builds the colour face colrink.py measures that no foundry made, into the
# directory it is given:
#
#   make hbcolrink
#
# shape/colrink.go measures a colour glyph's ink by painting it as HarfBuzz
# paints it for its extents, and the colour faces of the Google Fonts tree —
# against every glyph of which it is compared out of tree — paint with a few of
# COLR's thirty-two paint formats each. This face paints with all of them, a
# glyph each, and with each thing painting can do to a box:
#
#   - every transform, fixed and variable, around the origin and around a
#     centre, and nested one inside another;
#   - every fill, which paints the clip it is inside, and a fill with no clip
#     around it at all, which HarfBuzz finds unbounded and measures as nothing;
#   - PaintComposite in each of the modes that combine two groups differently;
#   - a layer list, a colour glyph painting another, one painting itself, and
#     a clip box, fixed and variable, on the glyph and on one it paints;
#   - a glyph clipped to an outline with no contours, whose void box
#     HarfBuzz's transform turns into a box a unit wide;
#   - a glyph clipped to a composite outline scaled through a 2x2, whose points
#     HarfBuzz transforms in floats;
#   - COLRv0 glyphs, whose layers are painted as solid fills;
#   - and a weight axis, along which every variable paint and the variable
#     clip box move, so that a face loaded away from its default paints with
#     the deltas.
#
# Its letters and marks are colour glyphs too, and it positions none of its
# marks and states no vertical metrics, so a string shaped in it has its marks
# placed and its glyphs hung by the painted boxes.
#
# BitmapInk.ttf is the bitmap table HarfBuzz asks before COLR: a CBLC and
# CBDT written byte by byte, since fontTools builds neither from nothing, with
# two strikes of which the larger is read — its ppem differing across and down
# — glyphs indexed by both of the index formats HarfBuzz reads and imaged in
# both of the image formats it reads metrics from, a glyph in a format it does
# not, one with no image data, and one only the smaller strike has. The images
# are placeholders: HarfBuzz reads the metrics in front of them and nothing
# else. Each glyph has an outline too, which is what a glyph the bitmaps do not
# answer for is measured by.
#
# It is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksum the expectations record stays
# true.
import os
import struct
import sys

from oracle import fonttools

fonttools()
from fontTools.colorLib.builder import buildCOLR, buildCPAL  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib.tables import otTables as ot  # noqa: E402
from fontTools.ttLib.tables.DefaultTable import DefaultTable  # noqa: E402
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402
from fontTools.varLib.builder import buildDeltaSetIndexMap  # noqa: E402
from fontTools.varLib.varStore import OnlineVarStoreBuilder  # noqa: E402

F = ot.PaintFormat


def poly(*pts):
    pen = TTGlyphPen(None)
    pen.moveTo(pts[0])
    for p in pts[1:]:
        pen.lineTo(p)
    pen.closePath()
    return pen.glyph()


def curve():
    # Quadratic curves whose off-curve points reach past the on-curve ones,
    # which the box of what is drawn takes in.
    pen = TTGlyphPen(None)
    pen.moveTo((100, 0))
    pen.qCurveTo((500, 50), (400, 400))
    pen.qCurveTo((0, 700), (100, 0))
    pen.closePath()
    return pen.glyph()


def glyph(name):
    return {"Format": F.PaintGlyph, "Glyph": name,
            "Paint": {"Format": F.PaintSolid, "PaletteIndex": 0, "Alpha": 1.0}}


def gradient_glyph(name, fmt):
    line = {"Extend": "pad", "ColorStop": [{"StopOffset": 0, "PaletteIndex": 0, "Alpha": 1.0},
                                           {"StopOffset": 1, "PaletteIndex": 1, "Alpha": 1.0}]}
    fills = {
        "linear": {"Format": F.PaintLinearGradient, "ColorLine": line,
                   "x0": 0, "y0": 0, "x1": 500, "y1": 500, "x2": 0, "y2": 500},
        "radial": {"Format": F.PaintRadialGradient, "ColorLine": line,
                   "x0": 100, "y0": 100, "r0": 0, "x1": 200, "y1": 200, "r1": 300},
        "sweep": {"Format": F.PaintSweepGradient, "ColorLine": line,
                  "centerX": 250, "centerY": 250, "startAngle": 0.0, "endAngle": 1.0},
        "varsolid": {"Format": F.PaintVarSolid, "PaletteIndex": 1, "Alpha": 0.5,
                     "VarIndexBase": var(-8192)},
    }
    return {"Format": F.PaintGlyph, "Glyph": name, "Paint": fills[fmt]}


def around(fmt, paint, **values):
    return dict({"Format": fmt, "Paint": paint}, **values)


SQ, TRI, BAR = "sq", "tri", "bar"

# The face varies along one axis, weight from 100 to 900 about 400, and each
# variable paint and the variable clip box is moved along it: var records a
# field's deltas — in its own units, an F2Dot14's in 1/16384ths and a Fixed's
# in 1/65536ths — at the heaviest end, and half of each the other way at the
# lightest, and names where they start, which is the paint's VarIndexBase.
# Every other location is interpolated between, so colrink.py asks at a
# location between two as well as at both ends.
DELTAS = []


def var(*deltas):
    base = len(DELTAS)
    DELTAS.extend(deltas)
    return base


def var_store():
    """The deltas var recorded, as a VarStore and the map from each field's
    index to its delta set."""
    builder = OnlineVarStoreBuilder(["wght"])
    builder.setSupports([{"wght": (0, 1.0, 1.0)}, {"wght": (-1.0, -1.0, 0)}])
    indices = [builder.storeDeltas([d, -(d // 2)]) for d in DELTAS]
    return builder.finish(optimize=False), buildDeltaSetIndexMap(indices)


COLOR = {
    # Fills, each clipped to an outline.
    "c_solid": glyph(SQ),
    "c_linear": gradient_glyph(TRI, "linear"),
    "c_radial": gradient_glyph("curve", "radial"),
    "c_sweep": gradient_glyph(BAR, "sweep"),
    "c_varsolid": gradient_glyph(SQ, "varsolid"),
    # A fill with nothing around it: unbounded.
    "c_unbounded": {"Format": F.PaintColrLayers, "Layers": [
        glyph(SQ), {"Format": F.PaintSolid, "PaletteIndex": 1, "Alpha": 1.0}]},
    # The transforms.
    "c_translate": around(F.PaintTranslate, glyph(TRI), dx=130, dy=-70),
    "c_vartranslate": around(F.PaintVarTranslate, glyph(TRI), dx=-40, dy=90,
                             VarIndexBase=var(60, -35)),
    "c_scale": around(F.PaintScale, glyph(TRI), scaleX=1.5, scaleY=0.75),
    "c_varscale": around(F.PaintVarScale, glyph(BAR), scaleX=0.5, scaleY=1.25,
                         VarIndexBase=var(4096, -2048)),
    "c_scalecentre": around(F.PaintScaleAroundCenter, glyph(SQ), scaleX=0.5, scaleY=1.5,
                            centerX=200, centerY=100),
    "c_varscalecentre": around(F.PaintVarScaleAroundCenter, glyph(SQ), scaleX=1.25, scaleY=0.25,
                               centerX=-100, centerY=0, VarIndexBase=var(2048, 4096, 50, -30)),
    "c_uniform": around(F.PaintScaleUniform, glyph(TRI), scale=0.3),
    "c_varuniform": around(F.PaintVarScaleUniform, glyph(TRI), scale=1.7, VarIndexBase=var(-3000)),
    "c_uniformcentre": around(F.PaintScaleUniformAroundCenter, glyph(BAR), scale=1.1,
                              centerX=0, centerY=250),
    "c_varuniformcentre": around(F.PaintVarScaleUniformAroundCenter, glyph(BAR), scale=0.6,
                                 centerX=333, centerY=-77, VarIndexBase=var(1500, -40, 25)),
    "c_rotate": around(F.PaintRotate, glyph(SQ), angle=0.25),
    "c_varrotate": around(F.PaintVarRotate, glyph(TRI), angle=-0.125, VarIndexBase=var(2048)),
    "c_rotatecentre": around(F.PaintRotateAroundCenter, glyph(BAR), angle=0.5, centerX=250,
                             centerY=250),
    "c_varrotatecentre": around(F.PaintVarRotateAroundCenter, glyph(SQ), angle=0.1, centerX=0,
                                centerY=400, VarIndexBase=var(-1024, 30, -60)),
    "c_skew": around(F.PaintSkew, glyph(SQ), xSkewAngle=0.1, ySkewAngle=-0.05),
    "c_varskew": around(F.PaintVarSkew, glyph(TRI), xSkewAngle=-0.2, ySkewAngle=0.0,
                        VarIndexBase=var(1000, 1500)),
    "c_skewcentre": around(F.PaintSkewAroundCenter, glyph(BAR), xSkewAngle=0.0, ySkewAngle=0.15,
                           centerX=100, centerY=0),
    "c_varskewcentre": around(F.PaintVarSkewAroundCenter, glyph(SQ), xSkewAngle=0.15,
                              ySkewAngle=0.1, centerX=300, centerY=200,
                              VarIndexBase=var(-500, 800, -70, 45)),
    "c_transform": around(F.PaintTransform, glyph(TRI), Transform={
        "xx": 0.8, "yx": 0.3, "xy": -0.2, "yy": 1.1, "dx": 55.5, "dy": -12.25}),
    "c_vartransform": around(F.PaintVarTransform, glyph(BAR), Transform={
        "xx": 1.0, "yx": 0.0, "xy": 0.5, "yy": 1.0, "dx": 0, "dy": 300,
        "VarIndexBase": var(0x4000, 0x2000, -0x6000, 0x1000, 25 << 16, -(15 << 16))}),
    "c_nested": around(F.PaintTranslate, around(F.PaintRotate, around(
        F.PaintScale, glyph("curve"), scaleX=0.5, scaleY=1.9), angle=0.3), dx=77, dy=33),
    # Layers, and composites of two groups.
    "c_layers": {"Format": F.PaintColrLayers, "Layers": [
        glyph(SQ), around(F.PaintTranslate, glyph(TRI), dx=600, dy=0), glyph(BAR)]},
    # Colour glyphs painting colour glyphs, and one with clip boxes.
    "c_colrglyph": {"Format": F.PaintColrGlyph, "Glyph": "c_translate"},
    "c_colrclipped": {"Format": F.PaintColrGlyph, "Glyph": "c_clipped"},
    "c_clipped": glyph(TRI),
    "c_varclipped": glyph(SQ),
    "c_self": {"Format": F.PaintColrLayers, "Layers": [
        around(F.PaintTranslate, {"Format": F.PaintColrGlyph, "Glyph": "c_self"}, dx=10, dy=5),
        glyph(BAR)]},
    # The outlines a fill can be clipped to.
    "c_empty": glyph("empty"),
    "c_emptymoved": around(F.PaintRotate, glyph("empty"), angle=0.25),
    "c_composite": glyph("comp"),
    # The letters and marks, painted.
    "A": around(F.PaintTranslate, glyph(TRI), dx=50, dy=0),
    "B": {"Format": F.PaintColrLayers, "Layers": [glyph(SQ), glyph(BAR)]},
    "acute": around(F.PaintTranslate, around(F.PaintScaleUniform, glyph(TRI), scale=0.25),
                    dx=-150, dy=650),
    "dotbelow": around(F.PaintTranslate, around(F.PaintScaleUniform, glyph(SQ), scale=0.2),
                       dx=-120, dy=-150),
}

MODES = ["CLEAR", "SRC", "DEST", "SRC_OVER", "DEST_OVER", "SRC_IN", "DEST_IN", "SRC_OUT",
         "DEST_OUT", "SRC_ATOP", "XOR", "MULTIPLY"]
for mode in MODES:
    COLOR["c_comp_" + mode.lower()] = {
        "Format": F.PaintComposite, "CompositeMode": mode,
        "SourcePaint": around(F.PaintTranslate, glyph(TRI), dx=250, dy=150),
        "BackdropPaint": glyph(SQ)}
# A composite whose source is unbounded, under the two modes whose answer
# depends on the source's.
for mode in ("SRC", "SRC_IN"):
    COLOR["c_comp_unbounded_" + mode.lower()] = {
        "Format": F.PaintComposite, "CompositeMode": mode,
        "SourcePaint": {"Format": F.PaintSolid, "PaletteIndex": 1, "Alpha": 1.0},
        "BackdropPaint": glyph(SQ)}

# COLRv0 glyphs: layers, each a solid fill of an outline.
V0 = {
    "v0_pair": [(SQ, 0), (TRI, 1)],
    "v0_one": [(BAR, 1)],
    "v0_empty": [("empty", 0)],
}

CLIPS = {"c_clipped": (-100, -50, 300, 400), "c_varclipped": (0, 0, 250, 250, var(-20, 10, 35, 60))}

OUTLINES = {
    ".notdef": poly((50, 0), (50, 700), (450, 700), (450, 0)),
    "space": Glyph(),
    "empty": Glyph(),
    SQ: poly((0, 0), (0, 400), (400, 400), (400, 0)),
    TRI: poly((0, -20), (250, 480), (500, -20)),
    BAR: poly((-60, 120), (-60, 180), (620, 180), (620, 120)),
    "curve": curve(),
}


def composite(with_matched):
    # Components: one through a 2x2 and moved; one scaled with its offset
    # scaled too, which the flag SCALED_COMPONENT_OFFSET asks for and HarfBuzz
    # honours; and, where with_matched says, one placed by matching its first point
    # to the third point already gathered.
    c = GlyphComponent()
    c.glyphName, c.x, c.y, c.flags = SQ, 37, -11, 0
    c.transform = [[0.7, 0.2], [-0.3, 0.9]]
    scaled = GlyphComponent()
    scaled.glyphName, scaled.x, scaled.y, scaled.flags = TRI, 60, 40, 0x0800
    scaled.transform = [[1.5, 0], [0, 0.5]]
    matched = GlyphComponent()
    matched.glyphName, matched.firstPt, matched.secondPt, matched.flags = BAR, 2, 0, 0
    g = Glyph()
    g.numberOfContours = -1
    g.components = [c, scaled, matched] if with_matched else [c, scaled]
    return g


def build(path, variable):
    """ColourInk, variable along its weight axis; or, not variable,
    ColourInkStatic, whose composite has a component placed by matching
    points, which LoadInstance cannot instance and so is not in the variable
    face."""
    order = list(OUTLINES) + ["comp"] + sorted(COLOR) + sorted(V0)
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x20: "space", 0x41: "A", 0x42: "B", 0x301: "acute", 0x323: "dotbelow"})
    glyf = dict(OUTLINES)
    glyf["comp"] = composite(not variable)
    # A colour glyph's own outline is what a reader with no colour draws; it is
    # a small box here, so that the painted box and the outline's differ.
    for name in list(COLOR) + list(V0):
        glyf[name] = poly((10, 10), (10, 60), (60, 60), (60, 10))
    fb.setupGlyf(glyf)
    metrics = {name: (600, 0) for name in order}
    metrics["acute"] = metrics["dotbelow"] = (0, 10)
    fb.setupHorizontalMetrics(metrics)
    fb.setupHorizontalHeader(ascent=900, descent=-200)
    fb.setupOS2(sTypoAscender=900, sTypoDescender=-200, usWinAscent=900, usWinDescent=200)
    fb.setupNameTable({"familyName": "ColourInk", "styleName": "Regular"})
    fb.setupPost()
    glyphs = dict(COLOR)
    glyphs.update(V0)
    # The variations are added to the table after it is built, since given to
    # buildCOLR they would have it write the COLRv0 glyphs as COLRv1 ones.
    colr = buildCOLR(glyphs, glyphMap=fb.font.getReverseGlyphMap(), clipBoxes=CLIPS)
    if variable:
        colr.table.VarStore, colr.table.VarIndexMap = var_store()
        fb.setupFvar([("wght", 100, 400, 900, "Weight")], [])
    fb.font["COLR"] = colr
    fb.font["CPAL"] = buildCPAL([[(1.0, 0.0, 0.0, 1.0), (0.0, 0.0, 1.0, 1.0)]])
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def cblc_cbdt():
    """A CBLC and a CBDT: two strikes, the larger of which HarfBuzz reads."""
    cbdt = bytearray(struct.pack(">HH", 3, 0))

    def image(fmt, metrics):
        at = len(cbdt)
        if fmt == 17:
            cbdt.extend(struct.pack(">BBbbB", *metrics))
        elif fmt == 18:
            cbdt.extend(struct.pack(">BBbbBbbB", *metrics))
        png = b"\x89PNG placeholder"
        cbdt.extend(struct.pack(">I", len(png)) + png)
        return at

    def subtable(index_fmt, image_fmt, images):
        # images are CBDT offsets, one more than the glyphs, relative to the
        # first; an equal pair is a glyph with no image.
        base = images[0]
        rel = [o - base for o in images]
        if index_fmt == 1:
            body = struct.pack(">HHI", 1, image_fmt, base) + b"".join(struct.pack(">I", r) for r in rel)
        else:
            body = struct.pack(">HHI", 3, image_fmt, base) + b"".join(struct.pack(">H", r) for r in rel)
        return body + b"\0" * (-len(body) % 4)

    def strike(records, ppem_x, ppem_y, first, last):
        # records: (first glyph, last glyph, subtable bytes).
        array = b""
        tables = b""
        offset = 8 * len(records)
        for f, l, body in records:
            array += struct.pack(">HHI", f, l, offset + len(tables))
            tables += body
        line = struct.pack(">bbBbbbbbbbbb", 100, -20, 120, 0, 0, 0, 0, 0, 0, 0, 0, 0)
        return array + tables, lambda at: (
            struct.pack(">IIII", at, len(array + tables), len(records), 0) + line + line +
            struct.pack(">HHBBBb", first, last, ppem_x, ppem_y, 32, 1))

    # The larger strike: glyphs 1 and 2 small-metric images through index
    # format 1, glyph 3 with no image, glyphs 4 and 5 big-metric images through
    # index format 3, and glyph 6 in a format HarfBuzz reads no metrics from.
    g1 = image(17, (109, 117, -3, 101, 120))
    g2 = image(17, (64, 30, 7, -9, 40))
    g3 = len(cbdt)
    g4 = image(18, (100, 90, 12, 88, 95, 0, 0, 100))
    g5 = image(18, (1, 255, -128, 127, 255, 0, 0, 1))
    end5 = len(cbdt)
    g6 = image(19, None)
    end6 = len(cbdt)
    # And glyph 5 again, in a record after the one that has it, which
    # HarfBuzz never reaches: it reads the first record whose range holds a
    # glyph.
    g5again = image(17, (10, 10, 1, 1, 12))
    end5again = len(cbdt)
    big, big_header = strike([
        (1, 3, subtable(1, 17, [g1, g2, g3, g3])),
        (4, 5, subtable(3, 18, [g4, g5, end5])),
        (6, 6, subtable(1, 19, [g6, end6])),
        (5, 5, subtable(1, 17, [g5again, end5again])),
    ], 136, 128, 1, 6)
    # The smaller strike, which alone has glyph 7.
    g7 = image(17, (20, 20, 0, 18, 22))
    small, small_header = strike([(7, 7, subtable(1, 17, [g7, len(cbdt)]))], 20, 20, 7, 7)
    at_big = 8 + 48 * 2
    at_small = at_big + len(big)
    cblc = struct.pack(">HHI", 3, 0, 2) + small_header(at_small) + big_header(at_big) + big + small
    return cblc, bytes(cbdt)


def bitmap(path):
    order = [".notdef"] + ["bm%d" % i for i in range(1, 9)]
    fb = FontBuilder(2048, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41 + i: g for i, g in enumerate(order[1:])})
    fb.setupGlyf({g: poly((10 * i, 0), (10 * i, 300), (400, 300), (400, 0)) for i, g in enumerate(order)})
    fb.setupHorizontalMetrics({g: (1200, 10 * i) for i, g in enumerate(order)})
    fb.setupHorizontalHeader(ascent=1900, descent=-500)
    fb.setupOS2(sTypoAscender=1900, sTypoDescender=-500, usWinAscent=1900, usWinDescent=500)
    fb.setupNameTable({"familyName": "BitmapInk", "styleName": "Regular"})
    fb.setupPost()
    cblc, cbdt = cblc_cbdt()
    for tag, data in (("CBLC", cblc), ("CBDT", cbdt)):
        fb.font[tag] = raw(tag, data)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def raw(tag, data):
    """A table fontTools writes as the bytes it is given."""
    t = DefaultTable(tag)
    t.data = data
    return t


build(os.path.join(sys.argv[1], "ColourInk.ttf"), True)
build(os.path.join(sys.argv[1], "ColourInkStatic.ttf"), False)
bitmap(os.path.join(sys.argv[1], "BitmapInk.ttf"))
