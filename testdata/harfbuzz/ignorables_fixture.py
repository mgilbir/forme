# Builds the face ignorables.txt is shaped in, into the directory it is given:
#
#   make hbshaping
#
# Ignorables.ttf is the rules a font writes about the characters nothing is
# drawn for, and about marks on a ligature built from a multiple substitution
# — the last shaping differences the Google Fonts sweep found, each in one
# face and each with a mechanism no committed corpus exercised:
#
#   - 'ccmp' draws a dotless i before a mark above, and lists the combining
#     grapheme joiner's glyph among the marks, as Gentium Book Plus does: a
#     joiner at the end of the run, or one that kept two marks apart, is not
#     stepped over by a substitution and is matched there.
#   - 'liga' makes an f_i ligature, which a zero width non-joiner between the
#     two stops and every other invisible character does not.
#   - 'liga' makes a flag from U+1F3F4 and the tag characters that spell
#     England, as an emoji font does, which no substitution may step over.
#   - 'ccmp' joins an acute and a patah, in either order, into one mark,
#     which a combining grapheme joiner between them stops where it kept the
#     patah from being reordered before the acute, and does not where there
#     was nothing to keep.
#   - 'ccmp' gives the zero width space a glyph of its own, and 'liga' makes an
#     a_b ligature: a soft hyphen between the two is stepped over, and the
#     space, once a substitution has touched it, is the font's and is not.
#   - 'init' joins a Mongolian letter's initial form with the free variation
#     selector after it, which it can only do if the selector has taken the
#     letter's form.
#   - 'ccmp' takes U+FB1F apart into yod, yod and patah, and 'liga' joins a vav
#     before it with the first yod, as Handjet does; then a mark-to-base lookup
#     puts the patah on the second yod and a mark-to-ligature lookup on the
#     ligature — which mark-to-ligature reaches by stepping over the second yod,
#     a later part of the multiple substitution.
#
# And the corpus writes the characters a substitution may not step over —
# a tag, a Mongolian free variation selector — between f and i, where no rule
# names them, and the Duployan shorthand format control, which HarfBuzz does
# not count among the characters nothing is drawn for at all.
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
from fontTools.ttLib.tables._g_l_y_f import Glyph  # noqa: E402


def rect(x0, y0, x1, y1):
    pen = TTGlyphPen(None)
    pen.moveTo((x0, y0))
    pen.lineTo((x0, y1))
    pen.lineTo((x1, y1))
    pen.lineTo((x1, y0))
    pen.closePath()
    return pen.glyph()


TAGS = {0xE0062: "tag_b", 0xE0065: "tag_e", 0xE0067: "tag_g", 0xE006E: "tag_n", 0xE007F: "tag_cancel"}
CMAP = {
    0x20: "space", 0x61: "a", 0x62: "b", 0x66: "f", 0x69: "i",
    0x0301: "acutecomb", 0x034F: "cgj", 0x00AD: "shy", 0x200B: "zwsp", 0x200C: "zwnj",
    0x200D: "zwj", 0x1F3F4: "blackflag",
    0x05D5: "vav", 0x05D9: "yod", 0x05B7: "patah", 0x05BC: "dagesh", 0xFB1F: "yodyodpatah",
    0x1820: "mong_a", 0x180B: "fvs1",
}
CMAP.update(TAGS)
ORDER = [".notdef"] + sorted(set(CMAP.values())) + [
    "dotlessi", "f_i", "flag_gbeng", "vav_yod", "zwsp.alt", "a_b", "mong_a.init", "mong_a.fvs1", "acute_patah"]
MARKS = {"acutecomb", "cgj", "patah", "dagesh", "acute_patah"}

FEATURES = """
languagesystem DFLT dflt;
languagesystem latn dflt;
languagesystem hebr dflt;
languagesystem mong dflt;

@MARKS_ABOVE = [acutecomb cgj];

table GDEF {
    GlyphClassDef [space a b f i dotlessi vav yod blackflag shy zwsp zwnj zwj
                   tag_b tag_e tag_g tag_n tag_cancel yodyodpatah zwsp.alt mong_a
                   mong_a.init fvs1],
                  [f_i flag_gbeng vav_yod a_b mong_a.fvs1], [acutecomb cgj patah dagesh acute_patah], ;
} GDEF;

lookup DOTLESS {
    sub i by dotlessi;
} DOTLESS;

feature ccmp {
    sub i' lookup DOTLESS @MARKS_ABOVE;
    sub yodyodpatah by yod yod patah;
    # A lookup of its own, so that it is a single substitution, which keeps
    # the glyph it replaces and marks it substituted, rather than a multiple
    # one, which makes a new glyph.
    lookup ZWSP {
        sub zwsp by zwsp.alt;
    } ZWSP;
    sub acutecomb patah by acute_patah;
    sub patah acutecomb by acute_patah;
} ccmp;

feature init {
    lookup INIT {
        sub mong_a by mong_a.init;
    } INIT;
    lookup INIT_FVS {
        sub mong_a.init fvs1 by mong_a.fvs1;
    } INIT_FVS;
} init;

feature liga {
    sub f i by f_i;
    sub blackflag tag_g tag_b tag_e tag_n tag_g tag_cancel by flag_gbeng;
    sub a b by a_b;
    lookupflag IgnoreMarks;
    sub vav yod by vav_yod;
} liga;

markClass patah <anchor 0 -100> @BELOW;
markClass dagesh <anchor 0 300> @CENTRE;

feature mark {
    lookup BASE {
        pos base [vav yod] <anchor 150 0> mark @BELOW <anchor 150 300> mark @CENTRE;
    } BASE;
    lookup LIGATURE {
        pos ligature vav_yod <anchor 120 0> mark @BELOW <anchor 120 300> mark @CENTRE
            ligComponent <anchor 420 -20> mark @BELOW <anchor 420 280> mark @CENTRE;
    } LIGATURE;
} mark;
"""


def build(path):
    fb = FontBuilder(1000, isTTF=True)
    fb.setupGlyphOrder(ORDER)
    fb.setupCharacterMap(CMAP)
    glyphs, metrics = {}, {}
    for i, g in enumerate(ORDER):
        if g in ("space", "cgj", "shy", "zwsp", "zwsp.alt", "zwnj", "zwj", "fvs1") or g.startswith("tag_"):
            # Something to see: a glyph a rule names is drawn, and one it does
            # not is taken out, so each of these has an outline to show which.
            glyphs[g] = rect(0, 0, 40 + 10 * i, 40) if g != "space" else Glyph()
            metrics[g] = (0 if g in MARKS else 100 + i, 0)
            continue
        if g in MARKS:
            glyphs[g] = rect(-100, -150, -20, -80)
            metrics[g] = (0, -100)
            continue
        glyphs[g] = rect(50, 0, 250 + 10 * i, 600)
        metrics[g] = (400 + 20 * i, 50)
    fb.setupGlyf(glyphs)
    fb.setupHorizontalMetrics(metrics)
    fb.setupHorizontalHeader(ascent=800, descent=-200)
    fb.setupOS2(sTypoAscender=800, sTypoDescender=-200, usWinAscent=800, usWinDescent=200)
    fb.setupNameTable({"familyName": "Ignorables", "styleName": "Regular"})
    fb.setupPost()
    addOpenTypeFeaturesFromString(fb.font, FEATURES)
    fb.font["head"].created = fb.font["head"].modified = 3660681600
    fb.font.recalcTimestamp = False
    fb.save(path)


build(os.path.join(sys.argv[1], "Ignorables.ttf"))
