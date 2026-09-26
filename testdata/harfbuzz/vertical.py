# Shapes the vertical corpus with HarfBuzz, top to bottom, and writes what it
# produced — the glyphs, their advances down the page and their offsets, and
# each face's vertical metrics — so that shape/vertical_test.go can hold a run
# set upright to it.
#
#   make hbvertical
#
# The companion of shape.py, and the same bargain: the output is checked in, so
# running the test needs a Go toolchain and regenerating it needs Python and
# the pinned uharfbuzz.
#
# Each face is here for a different answer to where a glyph set upright is
# hung and how far it moves the pen:
#
#   NotoSans-Variable.ttf   no vmtx and no VORG: the line's height, and the ink
#                           centred in it (the bundled face)
#   NotoSansArabic.ttf      the same, for a script whose joining a vertical run
#                           does not apply
#   NotoSansJP-Regular.otf  CFF with vmtx and VORG, and a 'vert' (NOTO_CJK)
#   NotoSansJP-VF.ttf       TrueType with vmtx and no VORG: the top phantom
#                           point (NOTO_FONTS)
#   ipag.ttf                the same, from another foundry (NOTO_FONTS)
#   Unifont-Regular.otf     CFF with neither, and no 'vert': the vertical forms
#                           by character (NOTO_FONTS)
#   VerticalComposites.ttf  composites that take their origin from a component,
#                           and the positioning that moves glyphs down the page
#   VerticalFallbacks.ttf   no vertical metrics and no positioning: the typographic
#                           line height, marks placed by class, spaces, kern table
#   VerticalHhea.ttf        the line height from hhea
#
# The last three are built by vertical_fixture.py; see it for why each is there.
#
# Every string is shaped twice, top to bottom and across the page in the
# direction its script runs, because the second is what a run set sideways is
# shaped as and has to be what it was.
#
# The metrics are HarfBuzz's own answers for a glyph, asked directly —
# hb_font_get_glyph_v_advance and hb_font_get_glyph_v_origin — for every glyph
# the corpus reaches, every composite whose metrics come from a component, and
# every nth glyph besides, so that a face is sampled across its whole range
# and not only where the corpus happens to fall.
#
# Faces are named on the command line as NAME=PATH. A face whose file is not
# there stops the run rather than being left out: an expectation file that
# quietly covers fewer faces is one the Go test would pass on for less.
import hashlib
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
fonttools()
from fontTools.ttLib import TTFont  # noqa: E402

corpus_path, features_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
faces = [a.split("=", 1) for a in sys.argv[4:]]

lines = [l.rstrip("\n") for l in open(corpus_path, encoding="utf-8")]
lines = [l for l in lines if l != ""]
# The second corpus is a feature a document asks for, and the text: the rules
# that move a glyph along the page, which only a request turns on in a run set
# upright, and the ones that stop applying there. "-" asks for none.
featured = []
for l in open(features_path, encoding="utf-8"):
    l = l.rstrip("\n")
    if l:
        tag, _, text = l.partition(" ")
        featured.append(([] if tag == "-" else [tag], text))

# How many glyphs of each face are sampled beyond the ones the corpus reaches.
SAMPLE = 200
# USE_MY_METRICS, the composite flag that hands a glyph its component's
# metrics — and so, for a TrueType face with vmtx, its vertical origin.
USE_MY_METRICS = 0x0200


def shape(font, s, direction, tags=()):
    buf = hb.Buffer()
    buf.add_str(s)
    buf.guess_segment_properties()
    if direction:
        buf.direction = direction
    buf.flags = hb.BufferFlags.REMOVE_DEFAULT_IGNORABLES
    hb.shape(font, buf, {t: True for t in tags})
    return buf


def glyphs(buf):
    return " ".join(
        f"{i.codepoint},{p.x_advance},{p.y_advance},{p.x_offset},{p.y_offset}"
        for i, p in zip(buf.glyph_infos, buf.glyph_positions))


def composites_using_metrics(path):
    tt = TTFont(path, lazy=True)
    if "glyf" not in tt:
        return []
    glyf, order = tt["glyf"], tt.getGlyphOrder()
    out = []
    for gid, name in enumerate(order):
        g = glyf[name]
        if g.isComposite() and any(c.flags & USE_MY_METRICS for c in g.components):
            out.append(gid)
    return out


out = []
for name, path in faces:
    try:
        data = open(path, "rb").read()
    except FileNotFoundError:
        sys.exit(f"{name}: {path} is not there. Fetch the corpora it is in "
                 "(make notocjk noto-fonts) and run again.")
    face = hb.Face(data)
    font = hb.Font(face)
    out.append(f"face {name} {hashlib.sha256(data).hexdigest()}")
    reached = set()
    for s in lines:
        ttb = shape(font, s, "ttb")
        reached.update(i.codepoint for i in ttb.glyph_infos)
        out.append("V " + glyphs(ttb))
        out.append("H " + glyphs(shape(font, s, None)))
    for tags, s in featured:
        ttb = shape(font, s, "ttb", tags)
        reached.update(i.codepoint for i in ttb.glyph_infos)
        out.append("F " + glyphs(ttb))
    n = face.glyph_count
    sample = reached | set(range(0, n, max(1, n // SAMPLE))) | set(composites_using_metrics(path))
    for gid in sorted(sample):
        x, y = font.get_glyph_v_origin(gid)
        # HarfBuzz states the advance growing upwards, so down the page is
        # negative; the file states it as the length it is.
        out.append(f"M {gid} {-font.get_glyph_v_advance(gid)} {x} {y}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/vertical.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face, what HarfBuzz produces for each line of vertical.txt:\n")
    w.write("# V is the line shaped top to bottom and H the same line across the page,\n")
    w.write("# each glyph as index, x advance, y advance, x offset and y offset in\n")
    w.write("# font units; F is a line of vertical_features.txt shaped top to bottom\n")
    w.write("# with the feature it names; M is one glyph's vertical advance and origin.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# cases {len(lines)}\n")
    w.write(f"# featured {len(featured)}\n")
    for line in out:
        w.write(line + "\n")
