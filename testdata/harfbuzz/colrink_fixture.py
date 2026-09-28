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
# SbixInk.ttf is the bitmap table HarfBuzz asks before CBDT: an sbix written
# byte by byte, with a null strike, three sizes and two strikes of the largest,
# of which the first is read; PNG images whose IHDR states the box, a PNG
# wider than HarfBuzz reads, one too short to hold an IHDR, JPEG and TIFF
# images it does not read, and duplicates — of an image, of a duplicate, of
# itself, of a glyph the face does not have, one too short to name a glyph,
# and a chain one longer than HarfBuzz follows. Its strike is 62.5 units a
# pixel, so that the rounding of halves shows. SbixInkLarge.ttf is an sbix
# whose boxes run to millions of units, where HarfBuzz's single precision
# shows; SbixInkRejected.ttf, SbixInkOps.ttf and SbixInkOpsEdge.ttf are
# tables its sanitizer refuses, and one it takes by a byte. The images are
# placeholders behind their IHDR.
#
# It is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksum the expectations record stays
# true.
import os
import struct
import sys
import zlib

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
    points, which the variable face leaves out: it was built when
    LoadInstance could not instance one, and pointmatch_fixture.py's faces
    are where it is held to such components now."""
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


def png(width, height):
    """The front of a PNG: its signature and an IHDR chunk stating the size,
    which is all HarfBuzz reads of it, and then a placeholder where the image
    would be."""
    ihdr = b"IHDR" + struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    chunk = struct.pack(">I", 13) + ihdr + struct.pack(">I", zlib.crc32(ihdr))
    return b"\x89PNG\r\n\x1a\n" + chunk + b"placeholder"


def sbix_table(num_glyphs, strikes, offsets=None):
    """An sbix table. strikes are (ppem, {glyph: (x, y, type, data)}), or None
    for a null strike; a glyph a strike does not name has no data in it.
    offsets, where given, replaces the strike offsets the table would state,
    so that one can point where the strikes are not."""
    head = 8 + 4 * len(strikes)
    body = b""
    at = []
    for strike in strikes:
        if strike is None:
            at.append(0)
            continue
        ppem, glyphs = strike
        at.append(head + len(body))
        records = b""
        index = []
        start = 4 + 4 * (num_glyphs + 1)
        for gid in range(num_glyphs):
            index.append(start + len(records))
            if gid in glyphs:
                x, y, kind, data = glyphs[gid]
                records += struct.pack(">hh4s", x, y, kind) + data
        index.append(start + len(records))
        body += struct.pack(">HH", ppem, 72) + b"".join(struct.pack(">I", o) for o in index) + records
    if offsets is not None:
        at = offsets
    return struct.pack(">HHI", 1, 1, len(at)) + b"".join(struct.pack(">I", o) for o in at) + body


def sbix_face(path, upem, glyphs, table):
    order = [".notdef"] + ["sb%d" % i for i in range(1, glyphs)]
    fb = FontBuilder(upem, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x41 + i: g for i, g in enumerate(order[1:26])})
    fb.setupGlyf({g: poly((5 * i, 0), (5 * i, 300), (400, 300), (400, 0)) for i, g in enumerate(order)})
    fb.setupHorizontalMetrics({g: (1200, 5 * i) for i, g in enumerate(order)})
    fb.setupHorizontalHeader(ascent=upem, descent=-upem // 4)
    fb.setupOS2(sTypoAscender=upem, sTypoDescender=-upem // 4, usWinAscent=upem, usWinDescent=upem // 4)
    fb.setupNameTable({"familyName": "SbixInk", "styleName": "Regular"})
    fb.setupPost()
    fb.font["sbix"] = raw("sbix", table)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def sbix(directory):
    """SbixInk.ttf, the bitmap table HarfBuzz asks before CBDT, and three
    faces whose sbix HarfBuzz reads differently from it."""
    # A duplicate's own offsets are not the ones its image is placed by, so
    # they are given ones the image it names does not have.
    dupe = lambda gid: (9, -9, b"dupe", struct.pack(">H", gid))  # noqa: E731
    n = 27
    # The strike HarfBuzz reads, asked at no size: the largest, and of two as
    # large the first. Upem 1000 at 16 ppem is 62.5 units a pixel, so an odd
    # number of pixels is a half unit, which HarfBuzz rounds up — towards
    # zero below it.
    chosen = {
        1: (-3, -5, b"png ", png(17, 13)),
        2: (32767, -32768, b"png ", png(1, 65535)),
        3: (0, 0, b"png ", png(65536, 10)),  # too wide: HarfBuzz reads no box
        4: dupe(1),  # the image of another glyph, with that glyph's offsets
        5: dupe(4),  # a duplicate of a duplicate
        6: dupe(6),  # a duplicate of itself, followed nine times and dropped
        7: (0, 0, b"dupe", b"\x01"),  # too short to name a glyph
        8: dupe(n + 3),  # a glyph the face does not have
        9: (1, 1, b"jpg ", b"\xff\xd8\xff placeholder"),  # HarfBuzz reads PNG only
        10: (1, 1, b"tiff", b"II*\x00 placeholder"),
        11: (7, -9, b"png ", b"\x89PNG short"),  # shorter than an IHDR: a box of nothing
        13: (0, 0, b"png ", b""),  # the record and no image
        14: (-32768, 32767, b"png ", png(65535, 1)),
        15: (3, 3, b"png ", png(0, 0)),
        16: dupe(15),
        # Eight duplicates in a row, which is as many as HarfBuzz follows, and
        # a ninth in front of them, which is one too many.
        **{g: dupe(g + 1) for g in range(17, 25)},
        25: (2, -2, b"png ", png(5, 6)),
        26: dupe(17),
    }
    # Glyph 12 only the smaller strikes have, and the equal strike after the
    # chosen one draws every glyph differently, so that reading it shows. The
    # first strike is null, whose size reads as zero.
    other = {g: (1, 1, b"png ", png(2, 2)) for g in range(1, n)}
    table = sbix_table(n, [
        None, (8, other), (16, chosen), (16, other), (12, other),
    ])
    sbix_face(os.path.join(directory, "SbixInk.ttf"), 1000, n, table)
    # 16384 units at 3 ppem, where a box a few thousand pixels across is
    # millions of units, past where a float counts in halves: HarfBuzz
    # rounds in single precision, which an odd count of units rounds to even,
    # and then takes each edge through a float, where past 2^24 units only
    # every other whole number is held — and a width of a quarter of a
    # billion units, measured as the difference of two such edges, only
    # every eighth.
    large = sbix_table(5, [(3, {1: (1538, -1538, b"png ", png(1538, 3073)),
                                 2: (-5121, 5121, b"png ", png(5121, 1)),
                                 3: (-24523, 0, b"png ", png(49102, 1))})])
    sbix_face(os.path.join(directory, "SbixInkLarge.ttf"), 16384, 5, large)
    # A strike offset past the end of the table, which fails HarfBuzz's
    # sanitizer and with it the whole table: every glyph is its outline.
    bad = sbix_table(4, [(16, {1: (0, 0, b"png ", png(4, 4))}), (8, {})])
    bad = bad[:12] + struct.pack(">I", len(bad) + 1) + bad[16:]
    sbix_face(os.path.join(directory, "SbixInkRejected.ttf"), 1000, 4, bad)
    # Two thousand strikes, the first few hundred sharing one strike's data
    # and the rest null. HarfBuzz's sanitizer allows a table checking of 64
    # units a byte of it, and checking costs a unit a byte checked: four for
    # each strike offset, and for each strike that is not null, four for each
    # of its glyph offsets. The live strikes and the padding after the table
    # are chosen so that the checking costs exactly what this table allows,
    # which is one unit too many — the allowance must be left above nothing —
    # and the sanitizer refuses the whole table; and a byte more of padding
    # allows sixty-four units more, and the same strikes pass.
    glyphs = 300
    shared = sbix_table(glyphs, [(16, {1: (0, 0, b"png ", png(4, 4))})])
    at = struct.unpack(">I", shared[8:12])[0]
    many = 2000
    head = 8 + 4 * many
    base = head + len(shared) - at
    live = 0
    while (4 * many + live * 4 * (glyphs + 1)) % 64 or 4 * many + live * 4 * (glyphs + 1) < 64 * base:
        live += 1
    pad = (4 * many + live * 4 * (glyphs + 1)) // 64 - base
    offsets = struct.pack(">I", head) * live + struct.pack(">I", 0) * (many - live)
    ops = struct.pack(">HHI", 1, 1, many) + offsets + shared[at:]
    sbix_face(os.path.join(directory, "SbixInkOps.ttf"), 1000, glyphs, ops + b"\0" * pad)
    sbix_face(os.path.join(directory, "SbixInkOpsEdge.ttf"), 1000, glyphs, ops + b"\0" * (pad + 1))


build(os.path.join(sys.argv[1], "ColourInk.ttf"), True)
build(os.path.join(sys.argv[1], "ColourInkStatic.ttf"), False)
bitmap(os.path.join(sys.argv[1], "BitmapInk.ttf"))
sbix(sys.argv[1])
