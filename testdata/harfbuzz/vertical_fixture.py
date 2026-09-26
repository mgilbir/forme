# Builds the three faces vertical.py shapes that no foundry made, into the
# directory it is given:
#
#   make hbvertical
#
# Each exists for rules none of the real faces exercises, so that no rule
# vertical.go reads is held to HarfBuzz only by its own reading of HarfBuzz.
#
# # VerticalComposites.ttf
#
# A TrueType face with vmtx and no VORG hangs each glyph from its top phantom
# point — the top of its box plus its top side bearing — and a composite takes
# that point from the last of its components flagged USE_MY_METRICS, followed
# down however deep the components nest. The Noto faces the vertical oracle is
# run over have no such composite, so none of them could say whether that rule
# is read the way HarfBuzz reads it. Nor do they give a run set upright the
# positioning that moves along the page rather than across it. This face does
# nothing else:
#
#   A, B        simple glyphs, with different tops and top side bearings
#   C1          B, then A flagged: A's point, not its own
#   C2          A flagged, then B flagged: the last one, B's
#   C3          C1 flagged: C1's, which is A's, two levels down
#   C4          A and B, neither flagged: its own point
#   space       no outline: its top side bearing alone
#   D, E        the same advance as each other at the end of the table, so
#               vhea declares fewer long records than there are glyphs and E's
#               side bearing is read from the short records after them
#   M, N        marks: M on a base and N on M, by anchors
#   comma       U+3001, and vcomma its vertical form U+FE11, which a run set
#               upright is drawn with only where the face has no 'vert'
#   beth        U+0712 SYRIAC LETTER BETH, with its joining forms, which a
#               Syriac word takes across the page and not down it
#
# and its rules are the ones a vertical run treats differently from a
# horizontal one:
#
#   vert        A to B, listed under 'kana' alone, so that a Latin run finds it
#               only by looking through the whole font — and having found it,
#               does not draw the comma in its vertical form
#   ltra        D to E, a left-to-right form, which a vertical run has none of
#   curs        D exits into E, which a run set upright joins down the page
#   vkrn, vpal  adjustments with a y advance, which only a vertical run applies
#   palt        one with an x advance, which a vertical run leaves alone
#   mark, mkmk  the marks, whose offsets a vertical run carries down the page
#
# # VerticalFallbacks.ttf
#
# A TrueType face with no vertical metrics and no positioning of its own, so
# that everything about an upright run in it is a fallback: the line's height
# for the advance, the ink centred in it for the origin, the marks placed by
# their combining classes and moved down the page with the pen, the widths of
# the spaces it has no glyph for, and a kern table that applies across the
# page and not down it. Its OS/2 asks for its typographic metrics to be used,
# and states them apart from hhea's and with a descender of the wrong sign,
# which is read as the magnitude it is.
#
# # VerticalHhea.ttf
#
# The same line height taken from hhea, for a face that does not ask for its
# typographic metrics: its hhea states a descender of the wrong sign too.
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
from fontTools.ttLib.tables._g_l_y_f import Glyph, GlyphComponent  # noqa: E402
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0  # noqa: E402

USE_MY_METRICS = 0x0200


def rect(x0, y0, x1, y1):
    pen = TTGlyphPen(None)
    pen.moveTo((x0, y0))
    pen.lineTo((x0, y1))
    pen.lineTo((x1, y1))
    pen.lineTo((x1, y0))
    pen.closePath()
    return pen.glyph()


def component(name, x, y, flags):
    c = GlyphComponent()
    c.glyphName, c.x, c.y, c.flags = name, x, y, flags
    return c


def composite(*components):
    g = Glyph()
    g.numberOfContours = -1
    g.components = list(components)
    return g


def finish(fb, path):
    # 2020-01-01, in the seconds since 1904 that head counts in.
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


def composites(path):
    order = [".notdef", "space", "A", "B", "C1", "C2", "C3", "C4", "M", "N", "D", "E",
             "comma", "vcomma", "beth", "beth.init", "beth.medi", "beth.fina"]
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap({0x20: "space", 0x41: "A", 0x42: "B", 0x43: "C1", 0x44: "C2",
                          0x45: "C3", 0x46: "C4", 0x47: "D", 0x48: "E",
                          0x0301: "M", 0x0302: "N", 0x3001: "comma", 0xFE11: "vcomma",
                          0x0712: "beth"})
    fb.setupGlyf({
        ".notdef": rect(50, 0, 450, 700),
        "space": Glyph(),
        "A": rect(100, -50, 500, 700),
        "B": rect(50, 100, 300, 450),
        "C1": composite(component("B", 0, 0, 0), component("A", 0, 100, USE_MY_METRICS)),
        "C2": composite(component("A", 0, 0, USE_MY_METRICS), component("B", 0, 300, USE_MY_METRICS)),
        "C3": composite(component("C1", 0, 50, USE_MY_METRICS)),
        "C4": composite(component("A", 0, 0, 0), component("B", 200, 200, 0)),
        "M": rect(-150, 750, -50, 850),
        "N": rect(-140, 900, -60, 980),
        "D": rect(0, 0, 200, 200),
        "E": rect(0, -100, 200, 600),
        "comma": rect(100, -100, 300, 100),
        "vcomma": rect(600, 600, 800, 800),
        "beth": rect(50, 0, 550, 400), "beth.init": rect(50, 0, 600, 400),
        "beth.medi": rect(0, 0, 600, 400), "beth.fina": rect(0, 0, 550, 400),
    })
    fb.setupHorizontalMetrics({
        ".notdef": (500, 50), "space": (250, 0), "A": (600, 100), "B": (350, 50),
        "C1": (600, 100), "C2": (600, 100), "C3": (600, 100), "C4": (700, 0),
        "M": (0, -150), "N": (0, -140), "D": (300, 0), "E": (300, 0),
        "comma": (1000, 100), "vcomma": (1000, 600),
        "beth": (600, 50), "beth.init": (600, 50), "beth.medi": (600, 0), "beth.fina": (600, 0),
    })
    fb.setupHorizontalHeader(ascent=880, descent=-120)
    fb.setupVerticalMetrics({
        ".notdef": (1000, 100), "space": (1000, 300), "A": (1000, 120), "B": (900, 200),
        "C1": (1000, 30), "C2": (1000, 40), "C3": (1000, -20), "C4": (1000, 60),
        "M": (0, 50), "N": (0, 20), "D": (800, 70), "E": (800, 90),
        "comma": (1000, 700), "vcomma": (1000, 80),
        "beth": (1000, 600), "beth.init": (1000, 600), "beth.medi": (1000, 600),
        "beth.fina": (1000, 600),
    })
    fb.setupVerticalHeader(ascent=500, descent=-500)
    fb.setupOS2(sTypoAscender=880, sTypoDescender=-120, usWinAscent=880, usWinDescent=120)
    fb.setupNameTable({"familyName": "VerticalComposites", "styleName": "Regular"})
    fb.setupPost()
    addOpenTypeFeaturesFromString(fb.font, """
languagesystem DFLT dflt;
languagesystem latn dflt;
languagesystem kana dflt;
languagesystem syrc dflt;

table GDEF {
    GlyphClassDef [space A B C1 C2 C3 C4 D E comma vcomma beth beth.init beth.medi beth.fina],
        , [M N], ;
} GDEF;

feature init {
    script syrc;
    language dflt;
    sub beth by beth.init;
} init;

feature medi {
    script syrc;
    language dflt;
    sub beth by beth.medi;
} medi;

feature fina {
    script syrc;
    language dflt;
    sub beth by beth.fina;
} fina;

feature vert {
    script kana;
    language dflt;
    sub A by B;
} vert;

feature ltra {
    sub D by E;
} ltra;

feature curs {
    pos cursive D <anchor NULL> <anchor 250 120>;
    pos cursive E <anchor 40 30> <anchor NULL>;
} curs;

feature vkrn {
    pos D E <0 0 40 -100>;
} vkrn;

feature vpal {
    pos [A B] <10 20 -50 -200>;
} vpal;

feature palt {
    pos [A B] <30 0 -60 0>;
} palt;

markClass [M] <anchor -100 750> @ABOVE;
markClass [N] <anchor -100 900> @STACK;

feature mark {
    pos base [A B D E] <anchor 300 720> mark @ABOVE;
} mark;

feature mkmk {
    pos mark M <anchor -100 860> mark @STACK;
} mkmk;
""")
    finish(fb, path)


def fallbacks(path):
    # The marks: above and below, the two double ones (combining classes 233
    # and 234), a spacing one of class zero that the pen still moves over, and
    # a Thai vowel sign, whose model places no marks and so leaves the pen's
    # cancelled advance on the offset.
    cmap = {0x20: "space", 0x6F: "o", 0x78: "x", 0x2E: "period", 0x30: "zero",
            0x0301: "acute", 0x0323: "dotbelow", 0x035C: "brevebelow",
            0x0361: "invbreve", 0x1CE1: "svarita", 0x0E01: "kokai", 0x0E31: "maihanakat"}
    order = [".notdef"] + sorted(cmap.values())
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap(cmap)
    fb.setupGlyf({
        ".notdef": rect(50, 0, 450, 700), "space": Glyph(),
        "o": rect(50, -10, 450, 480), "x": rect(20, 0, 480, 470),
        "period": rect(80, 0, 160, 80), "zero": rect(40, -10, 500, 710),
        "acute": rect(-250, 540, -120, 700), "dotbelow": rect(-200, -180, -120, -100),
        "brevebelow": rect(-300, -200, 100, -120), "invbreve": rect(-300, 560, 100, 640),
        "svarita": rect(20, 500, 180, 620), "kokai": rect(40, 0, 520, 500),
        "maihanakat": rect(-260, 540, -60, 640),
    })
    fb.setupHorizontalMetrics({
        ".notdef": (500, 50), "space": (230, 0), "o": (500, 50), "x": (500, 20),
        "period": (240, 80), "zero": (540, 40), "acute": (0, -250), "dotbelow": (0, -200),
        "brevebelow": (0, -300), "invbreve": (0, -300), "svarita": (200, 20),
        "kokai": (560, 40), "maihanakat": (0, -260),
    })
    fb.setupHorizontalHeader(ascent=1000, descent=-250)
    # fsSelection: REGULAR and USE_TYPO_METRICS.
    fb.setupOS2(version=4, sTypoAscender=900, sTypoDescender=300, sTypoLineGap=0,
                usWinAscent=1000, usWinDescent=250, fsSelection=0x40 | 0x80)
    fb.setupNameTable({"familyName": "VerticalFallbacks", "styleName": "Regular"})
    fb.setupPost()
    kern = newTable("kern")
    kern.version = 0
    sub = KernTable_format_0()
    sub.version, sub.coverage, sub.format = 0, 1, 0
    sub.kernTable = {("o", "x"): -80, ("x", "o"): -60}
    kern.kernTables = [sub]
    fb.font["kern"] = kern
    finish(fb, path)


def hhea(path):
    cmap = {0x20: "space", 0x6F: "o", 0x78: "x"}
    order = [".notdef"] + sorted(cmap.values())
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(order)
    fb.setupCharacterMap(cmap)
    fb.setupGlyf({".notdef": rect(50, 0, 450, 700), "space": Glyph(),
                  "o": rect(50, -10, 450, 480), "x": rect(20, 0, 480, 470)})
    fb.setupHorizontalMetrics({".notdef": (500, 50), "space": (230, 0), "o": (500, 50),
                               "x": (500, 20)})
    fb.setupHorizontalHeader(ascent=700, descent=300)
    # Typographic metrics that would give another answer, and no request that
    # they be used.
    fb.setupOS2(version=4, sTypoAscender=800, sTypoDescender=-100, sTypoLineGap=0,
                usWinAscent=700, usWinDescent=300, fsSelection=0x40)
    fb.setupNameTable({"familyName": "VerticalHhea", "styleName": "Regular"})
    fb.setupPost()
    finish(fb, path)


out = sys.argv[1]
composites(os.path.join(out, "VerticalComposites.ttf"))
fallbacks(os.path.join(out, "VerticalFallbacks.ttf"))
hhea(os.path.join(out, "VerticalHhea.ttf"))
