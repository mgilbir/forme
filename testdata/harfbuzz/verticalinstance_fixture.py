# Builds the faces verticalinstance.py sets upright that no foundry made, into
# the directory it is given:
#
#   make hbverticalinstance
#
# # VerticalVariable.ttf and VerticalVariableNoVVAR.ttf
#
# A variable TrueType face with vertical metrics, one weight axis from 100 to
# 900 whose default is 100, twice: once with VVAR and once without. At any
# other weight HarfBuzz advances a glyph set upright by vmtx's advance plus
# VVAR's delta where the face has VVAR, and by the distance between its
# vertical phantom points as gvar moves them where it does not; and it hangs
# the glyph from its top phantom point as gvar moves it, either way. The two
# faces give the two answers for the same glyphs, and the deltas are chosen so
# that they differ:
#
#   A        its outline, its right phantom point and both vertical ones move
#   B        its outline moves one way and its vertical phantom points the other
#   C        a composite of A, whose metrics it takes (USE_MY_METRICS), moved
#   D        nothing moves
#   space    no outline; its vertical phantom points alone move
#
# # VerticalKern.ttf
#
# A static face with a kern table of three subtables — one along a
# horizontal line, one down a vertical one, and one across a vertical one —
# and a 'vkrn' in GSUB that substitutes a glyph no text here uses. HarfBuzz
# applies a kern table's vertical subtables to a run set upright only where
# 'vkrn' is asked for and is somewhere in the face's layout tables, since the
# feature has no fallback of its own; this is the face in which it does, and
# VerticalKernNoVkrn.ttf, the same face with no 'vkrn', the one in which it
# does not. Both have a combining acute. The first does not position it, and a
# kern table that kerns across the line is one HarfBuzz leaves such a mark
# unplaced for, whether or not the kerning is asked for. The second attaches it
# to A in GPOS — which has no 'kern', so the kern table is still applied — and
# the table's cross-stream subtable ties the run together over the attachment
# whether or not kerning is asked for.
#
# Each is built with fontTools and its timestamps fixed, so that building it
# again produces the same bytes and the checksums the expectations record stay
# true.
import os
import sys

from oracle import fonttools

fonttools()
from fontTools.feaLib.builder import addOpenTypeFeaturesFromString  # noqa: E402
from fontTools.fontBuilder import FontBuilder  # noqa: E402
from fontTools.pens.ttGlyphPen import TTGlyphPen  # noqa: E402
from fontTools.ttLib import newTable  # noqa: E402
from fontTools.ttLib.tables import otTables as ot  # noqa: E402
from fontTools.ttLib.tables.TupleVariation import TupleVariation  # noqa: E402
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0  # noqa: E402
from fontTools.varLib import builder  # noqa: E402

USE_MY_METRICS = 0x0200


def rect(x0, y0, x1, y1):
    pen = TTGlyphPen(None)
    pen.moveTo((x0, y0))
    pen.lineTo((x0, y1))
    pen.lineTo((x1, y1))
    pen.lineTo((x1, y0))
    pen.closePath()
    return pen.glyph()


def finish(fb, path):
    # 2020-01-01, in the seconds since 1904 that head counts in.
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


ORDER = [".notdef", "space", "A", "B", "C", "D"]

# Deltas at the heaviest weight: each outline point's, then the four phantom
# points' — left, right, top, bottom — as gvar lists them.
DELTAS = {
    "A": [(0, 60)] * 4 + [(0, 0), (20, 0), (0, 45), (0, -25)],
    "B": [(0, -30)] * 4 + [(0, 0), (0, 0), (0, -10), (0, 15)],
    "C": [(0, 20)] + [(0, 0), (0, 0), (0, 7), (0, 3)],
    "space": [(0, 0), (0, 0), (0, 33), (0, -33)],
}

# VVAR's advance deltas at the heaviest weight, glyph by glyph.
VVAR_DELTAS = {".notdef": 0, "space": 2, "A": 37, "B": -11, "C": 0, "D": 5}


def variable(path, with_vvar):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(ORDER)
    fb.setupCharacterMap({0x20: "space", 0x41: "A", 0x42: "B", 0x43: "C", 0x44: "D"})
    c = Glyph()
    c.numberOfContours = -1
    comp = GlyphComponent()
    comp.glyphName, comp.x, comp.y, comp.flags = "A", 0, 50, USE_MY_METRICS
    c.components = [comp]
    fb.setupGlyf({
        ".notdef": rect(50, 0, 450, 700), "space": Glyph(),
        "A": rect(100, -50, 500, 700), "B": rect(50, 100, 300, 450),
        "C": c, "D": rect(0, 0, 200, 200),
    })
    fb.setupHorizontalMetrics({".notdef": (500, 50), "space": (250, 0), "A": (600, 100),
                               "B": (350, 50), "C": (600, 100), "D": (300, 0)})
    fb.setupHorizontalHeader(ascent=880, descent=-120)
    fb.setupVerticalMetrics({".notdef": (1000, 100), "space": (1000, 300), "A": (1000, 120),
                             "B": (900, 200), "C": (1000, 30), "D": (800, 70)})
    fb.setupVerticalHeader(ascent=500, descent=-500)
    fb.setupOS2(sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120)
    name = "VerticalVariable" if with_vvar else "VerticalVariableNoVVAR"
    fb.setupNameTable({"familyName": name, "styleName": "Thin"})
    fb.setupPost()
    fb.setupFvar([("wght", 100, 100, 900, "Weight")], [])
    fb.setupGvar({g: [TupleVariation({"wght": (0.0, 1.0, 1.0)}, list(d))] for g, d in DELTAS.items()})
    if with_vvar:
        regions = builder.buildVarRegionList([{"wght": (0.0, 1.0, 1.0)}], ["wght"])
        data = builder.buildVarData([0], [[VVAR_DELTAS[g]] for g in ORDER], optimize=False)
        vvar = ot.VVAR()
        vvar.Version = 0x00010000
        vvar.VarStore = builder.buildVarStore(regions, [data])
        vvar.AdvHeightMap = vvar.TsbMap = vvar.BsbMap = vvar.VOrgMap = None
        t = newTable("VVAR")
        t.table = vvar
        fb.font["VVAR"] = t
    finish(fb, path)


def kerned(path, vkrn):
    order = [".notdef", "space", "A", "B", "C", "Y", "Z", "acute"]
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x20: "space", 0x41: "A", 0x42: "B", 0x43: "C", 0x5A: "Z", 0x301: "acute"})
    fb.setupGlyf({".notdef": rect(50, 0, 450, 700), "space": Glyph(),
                  "A": rect(100, 0, 500, 700), "B": rect(50, 0, 450, 600),
                  "C": rect(80, 0, 420, 650), "Y": rect(0, 0, 100, 100), "Z": rect(0, 0, 200, 200),
                  "acute": rect(-250, 750, -150, 850)})
    fb.setupHorizontalMetrics({g: (600, 0) for g in order})
    fb.setupHorizontalHeader(ascent=880, descent=-120)
    fb.setupVerticalMetrics({g: (1000, 100) for g in order})
    fb.setupVerticalHeader(ascent=500, descent=-500)
    fb.setupOS2(sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120)
    fb.setupNameTable({"familyName": "VerticalKern", "styleName": "Regular"})
    fb.setupPost()
    if vkrn:
        addOpenTypeFeaturesFromString(fb.font, """
languagesystem DFLT dflt;
languagesystem latn dflt;

feature vkrn {
    sub Z by Y;
} vkrn;
""")
    else:
        addOpenTypeFeaturesFromString(fb.font, """
languagesystem DFLT dflt;
languagesystem latn dflt;

table GDEF {
    GlyphClassDef [A B C Y Z], , [acute], ;
} GDEF;

markClass acute <anchor -200 800> @TOP;

feature mark {
    pos base A <anchor 300 700> mark @TOP;
} mark;
""")
    kern = newTable("kern")
    kern.version = 0
    tables = []
    for coverage, pairs in (
        (1, {("A", "B"): -50, ("B", "C"): -30}),  # horizontal
        (0, {("A", "B"): -80, ("B", "C"): 40}),   # vertical
        (4, {("B", "C"): 30, ("C", "A"): -20}),   # vertical, across the line
    ):
        sub = KernTable_format_0()
        sub.version, sub.coverage, sub.format = 0, coverage, 0
        sub.kernTable = pairs
        tables.append(sub)
    kern.kernTables = tables
    fb.font["kern"] = kern
    finish(fb, path)


out = sys.argv[1]
variable(os.path.join(out, "VerticalVariable.ttf"), True)
variable(os.path.join(out, "VerticalVariableNoVVAR.ttf"), False)
kerned(os.path.join(out, "VerticalKern.ttf"), True)
kerned(os.path.join(out, "VerticalKernNoVkrn.ttf"), False)
