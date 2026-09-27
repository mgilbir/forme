# Builds VariedLayout.ttf and VariedLayoutTypo.ttf, the faces instancevaried.py
# asks HarfBuzz about, and VariedAxes.ttf, which layout's tests set, into the
# directory it is given:
#
#   make hbinstancevaried
#
# A variable TrueType face, one weight axis from 100 to 900 whose default is
# 400, that states everything a location moves besides the outlines:
#
#   MVAR     a delta for every value tag the table defines but gasp's, each
#            its own, so that a tag written to the wrong field shows
#   fvar     two named instances, one of them named in three languages
#   GPOS     Device tables that are VariationIndexes, on each kind of record
#            that can carry one: a single adjustment (format 1 and 2), a pair
#            listed glyph by glyph and a pair stated by class, a mark's anchor
#            and a base's, a ligature component's, a mark on a mark, and a
#            cursive entry and exit
#
# with a GDEF 1.3 item variation store for the second. The deltas are chosen
# so that the rounding matters: several land on a half unit at the
# intermediate weights, and some of those are negative, where HarfBuzz's
# rounding (half up) and the C library's (half away from zero) part.
#
# hhea's ascender, descender and line gap are not OS/2's typographic three,
# and VariedLayoutTypo.ttf is the same face with USE_TYPO_METRICS set: HarfBuzz
# reads the first three from hhea in one and from OS/2 in the other, and moves
# them by MVAR's 'hasc', 'hdsc' and 'hlgp' either way.
#
# It is built with fontTools and its timestamps fixed, so that building it again
# produces the same bytes and the checksum the expectations record stays true.
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
from fontTools.varLib import builder  # noqa: E402


def rect(x0, y0, x1, y1):
    pen = TTGlyphPen(None)
    pen.moveTo((x0, y0))
    pen.lineTo((x0, y1))
    pen.lineTo((x1, y1))
    pen.lineTo((x1, y0))
    pen.closePath()
    return pen.glyph()


ORDER = [".notdef", "space", "A", "B", "C", "D", "E", "F", "G", "H", "fi", "acute", "grave"]
CMAP = {0x20: "space", 0x41: "A", 0x42: "B", 0x43: "C", 0x44: "D", 0x45: "E", 0x46: "F",
        0x47: "G", 0x48: "H", 0xFB01: "fi", 0x301: "acute", 0x300: "grave"}

# MVAR: each tag's delta at the lightest and at the heaviest weight. Odd
# numbers, so that the weights between land on halves.
MVAR = {
    "hasc": (-15, 31), "hdsc": (9, -23), "hlgp": (5, 13), "hcla": (-7, 29), "hcld": (3, 21),
    "hcrs": (0, -3), "hcrn": (0, 7), "hcof": (-5, 11),
    "vasc": (-11, 17), "vdsc": (13, -19), "vlgp": (3, 9), "vcrs": (1, 5), "vcrn": (-3, 3), "vcof": (7, -9),
    "xhgt": (-21, 37), "cpht": (-17, 25),
    "sbxs": (-9, 15), "sbys": (-7, 13), "sbxo": (5, -11), "sbyo": (3, 9),
    "spxs": (-5, 17), "spys": (-3, 19), "spxo": (7, -13), "spyo": (-9, 23),
    "strs": (-11, 27), "stro": (-13, 21),
    "unds": (-7, 15), "undo": (5, -19),
}

FEATURES = """
languagesystem DFLT dflt;
languagesystem latn dflt;

table GDEF {
    GlyphClassDef [A B C D E F G H space], [fi], [acute grave], ;
} GDEF;

@LEFT = [C D];
@RIGHT = [E F];

markClass acute <anchor (wght=100:-205 wght=400:-200 wght=900:-187) (wght=100:790 wght=400:800 wght=900:833)> @TOP;
markClass grave <anchor (wght=100:-190 wght=400:-200 wght=900:-213) 800> @TOP;

feature kern {
    pos A B (wght=100:-37 wght=400:-50 wght=900:-75);
    pos B A <(wght=100:3 wght=400:0 wght=900:-9) 0 (wght=100:-13 wght=400:-20 wght=900:-41) 0>;
    pos @LEFT @RIGHT (wght=100:-9 wght=400:-30 wght=900:-55);
} kern;

feature dist {
    pos G (wght=100:21 wght=400:40 wght=900:67);
    pos [H E] <(wght=100:5 wght=400:0 wght=900:-11) (wght=100:-3 wght=400:0 wght=900:15) (wght=100:9 wght=400:0 wght=900:25) 0>;
} dist;

feature mark {
    pos base [A B C D E F G H] <anchor (wght=100:290 wght=400:300 wght=900:327) (wght=100:690 wght=400:700 wght=900:751)> mark @TOP;
    pos ligature fi <anchor (wght=100:140 wght=400:150 wght=900:163) 700> mark @TOP
        ligComponent <anchor (wght=100:430 wght=400:450 wght=900:489) (wght=100:680 wght=400:700 wght=900:719)> mark @TOP;
} mark;

feature mkmk {
    pos mark acute <anchor (wght=100:-210 wght=400:-200 wght=900:-179) (wght=100:880 wght=400:900 wght=900:947)> mark @TOP;
} mkmk;

feature curs {
    pos cursive D <anchor 0 (wght=100:190 wght=400:200 wght=900:229)> <anchor (wght=100:575 wght=400:600 wght=900:637) (wght=100:290 wght=400:300 wght=900:341)>;
    pos cursive E <anchor 0 (wght=100:210 wght=400:200 wght=900:171)> <anchor 600 300>;
} curs;
"""


def build(path, typo):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(ORDER)
    fb.setupCharacterMap(CMAP)
    glyphs = {g: rect(50, 0, 550, 700) for g in ORDER}
    glyphs["space"] = rect(0, 0, 0, 0)
    glyphs["acute"] = rect(-250, 750, -150, 850)
    glyphs["grave"] = rect(-250, 750, -150, 850)
    glyphs["fi"] = rect(50, 0, 950, 700)
    fb.setupGlyf(glyphs)
    metrics = {g: (600, 50) for g in ORDER}
    metrics["fi"] = (1000, 50)
    metrics["acute"] = metrics["grave"] = (0, -250)
    fb.setupHorizontalMetrics(metrics)
    fb.setupHorizontalHeader(ascent=880, descent=-120, lineGap=40, caretSlopeRise=1, caretSlopeRun=0, caretOffset=0)
    fb.setupVerticalMetrics({g: (1000, 100) for g in ORDER})
    fb.setupVerticalHeader(ascent=500, descent=-500, lineGap=0, caretSlopeRise=0, caretSlopeRun=1, caretOffset=0)
    fb.setupOS2(version=4, sTypoAscender=860, sTypoDescender=-140, sTypoLineGap=60,
                usWinAscent=900, usWinDescent=150, sxHeight=500, sCapHeight=700,
                ySubscriptXSize=650, ySubscriptYSize=600, ySubscriptXOffset=0, ySubscriptYOffset=75,
                ySuperscriptXSize=650, ySuperscriptYSize=600, ySuperscriptXOffset=0, ySuperscriptYOffset=350,
                yStrikeoutSize=50, yStrikeoutPosition=300,
                fsSelection=0x0040 | (0x0080 if typo else 0))
    fb.setupNameTable({"familyName": "VariedLayoutTypo" if typo else "VariedLayout", "styleName": "Regular"})
    fb.setupPost(underlinePosition=-100, underlineThickness=50)
    # Two named instances, one with its subfamily name localized into French
    # and Japanese: what CSS's font-named-instance descriptor asks for, by any
    # of its names (shape.Face.NamedInstance).
    fb.setupFvar([("wght", 100, 400, 900, "Weight")], [
        {"location": {"wght": 700}, "stylename": {"en": "Bold", "fr": "Gras", "ja": "\u592a\u5b57"}},
        {"location": {"wght": 250}, "stylename": "Book"},
    ])
    fb.setupGvar({})
    addOpenTypeFeaturesFromString(fb.font, FEATURES)

    tags = sorted(MVAR)
    regions = builder.buildVarRegionList([{"wght": (-1.0, -1.0, 0.0)}, {"wght": (0.0, 1.0, 1.0)}], ["wght"])
    data = builder.buildVarData([0, 1], [list(MVAR[t]) for t in tags], optimize=False)
    store = builder.buildVarStore(regions, [data])
    mvar = ot.MVAR()
    mvar.Version = 0x00010000
    mvar.Reserved = 0
    mvar.VarStore = store
    mvar.ValueRecord = []
    for i, t in enumerate(tags):
        rec = ot.MetricsValueRecord()
        rec.ValueTag = t
        rec.VarIdx = i
        mvar.ValueRecord.append(rec)
    table = newTable("MVAR")
    table.table = mvar
    fb.font["MVAR"] = table

    # 2020-01-01, in the seconds since 1904 that head counts in.
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def axes(path):
    """VariedAxes.ttf: five axes and no deltas, for the layout tests of where
    CSS Fonts 4 §7.2 places a face in its design space (layout's
    fontinstance_test.go) — weight, width, optical size, slant and italic,
    each with its default somewhere a test can tell from its ends."""
    order = [".notdef", "space", "A", "B"]
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x20: "space", 0x41: "A", 0x42: "B"})
    fb.setupGlyf({g: rect(50, 0, 550, 700) for g in order})
    fb.setupHorizontalMetrics({g: (600, 50) for g in order})
    fb.setupHorizontalHeader(ascent=880, descent=-120)
    fb.setupOS2(version=4, sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120,
                sxHeight=500, sCapHeight=700)
    fb.setupNameTable({"familyName": "VariedAxes", "styleName": "Regular", "psName": "VariedAxes-Regular"})
    fb.setupPost()
    fb.setupFvar([("wght", 100, 400, 900, "Weight"), ("wdth", 50, 100, 150, "Width"),
                  ("opsz", 8, 14, 144, "Optical size"), ("slnt", -15, 0, 0, "Slant"),
                  ("ital", 0, 0, 1, "Italic")], [
        {"location": {"wght": 700, "wdth": 100, "opsz": 14, "slnt": 0, "ital": 0}, "stylename": "Bold"},
        {"location": {"wght": 300, "wdth": 75, "opsz": 72, "slnt": 0, "ital": 0},
         "stylename": "Display Light Condensed"},
    ])
    fb.setupGvar({})
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


if __name__ == "__main__":
    build(os.path.join(sys.argv[1], "VariedLayout.ttf"), False)
    build(os.path.join(sys.argv[1], "VariedLayoutTypo.ttf"), True)
    axes(os.path.join(sys.argv[1], "VariedAxes.ttf"))
