# Asks fontTools' subsetter which subroutines of a name-keyed CFF a subset
# keeps, and writes the answers, so that shape/cffsubrs_test.go can hold the
# subsetter in shape/cffsubset.go to them.
#
#   make cffsubrs
#
# The same bargain as every oracle here: the output is checked in, so running
# the test needs a Go toolchain and regenerating it needs Python and the pinned
# fontTools.
#
# What is asked is exactly the question the subsetter answers: for a set of
# glyphs, which of the font's global and local subroutines some kept glyph
# reaches, however deeply. fontTools' subsetter is run for real — with
# desubroutinize off, so it keeps and renumbers subroutines rather than
# inlining them, and with the layout closure off, so the glyphs kept are the
# ones asked for and the seac components the CFF closure adds to them. Which of
# the original subroutines survived is read back by identity: fontTools keeps
# the surviving subroutine objects and drops the rest, so an object that is
# still in an INDEX afterwards is one it kept.
#
# The faces are named on the command line as NAME=PATH:
#
#   CFFInk.otf                built by cffink_fixture.py, with global and local
#                             subroutines and seacs, every glyph on its own and
#                             in a few sets
#   SourceSans3-Regular.otf   the static Source fonts (make cff-fonts), real
#   SourceSerif4-Regular.otf  name-keyed CFF built by a subroutinizer, in sets
#                             from a word to the whole font
#
# A face whose file is not there stops the run rather than being left out.
import hashlib
import io
import random
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
ft = fonttools()
from fontTools import subset  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402

out_path = sys.argv[1]
faces = [a.split("=", 1) for a in sys.argv[2:]]

TEXTS = [
    "Hamburgefonstiv",
    "The quick brown fox jumps over the lazy dog. 0123456789",
    "Ångström été naïve ß",
]


def survivors(data, gids):
    """The glyphs fontTools keeps for gids, and the original indices of the
    global and local subroutines it keeps with them."""
    font = TTFont(io.BytesIO(data))
    order = font.getGlyphOrder()
    cff = font["CFF "].cff
    top = cff.topDictIndex[0]
    if hasattr(top, "ROS"):
        sys.exit("a CID-keyed face: this oracle is for name-keyed CFF")
    gsubrs = cff.GlobalSubrs
    lsubrs = getattr(top.Private, "Subrs", None)
    ids = {}
    for kind, subrs in (("g", gsubrs), ("l", lsubrs)):
        if subrs is None:
            continue
        for i in range(len(subrs)):
            ids[id(subrs[i])] = (kind, i)
    opts = subset.Options()
    opts.desubroutinize = False
    opts.layout_features = []
    opts.layout_closure = False
    opts.notdef_outline = True
    opts.glyph_names = True
    opts.name_IDs = ["*"]
    s = subset.Subsetter(options=opts)
    s.populate(gids=gids)
    s.subset(font)
    kept = sorted(order.index(g) for g in s.glyphs_retained)
    cff = font["CFF "].cff
    top = cff.topDictIndex[0]
    got = {"g": [], "l": []}
    for subrs in (cff.GlobalSubrs, getattr(top.Private, "Subrs", None)):
        if subrs is None:
            continue
        for i in range(len(subrs)):
            kind, orig = ids[id(subrs[i])]
            got[kind].append(orig)
    return kept, sorted(got["g"]), sorted(got["l"])


def readable(data, g):
    """Whether fontTools can run glyph g at all. The fixture carries glyphs
    built to be malformed — a hintmask with no room for its mask, a call to a
    subroutine that is not there — which HarfBuzz reads as having no ink and
    fontTools' decompiler refuses outright, so there is no answer of its to
    hold anything to."""
    try:
        survivors(data, [g])
        return True
    except Exception:
        return False


def cases(name, font, data, n):
    """The glyph sets each face is asked about."""
    if name == "CFFInk.otf":
        ok = [g for g in range(1, n) if readable(data, g)]
        for g in ok:
            yield f"glyph{g}", [g]
        # Not the odd-numbered glyphs together, which were asked: fontTools'
        # own renumbering fails on that set (a ValueError in
        # subset_subroutines), so it has no answer to give.
        yield "all", ok
        return
    cmap = font.getBestCmap()
    order = font.getGlyphOrder()
    for i, text in enumerate(TEXTS):
        yield f"text{i}", sorted({order.index(cmap[ord(c)]) for c in text if ord(c) in cmap})
    rng = random.Random(name)
    for i in range(8):
        yield f"one{i}", [rng.randrange(1, n)]
    yield "every17", list(range(0, n, 17))
    yield "all", list(range(n))


out = []
for name, path in faces:
    try:
        data = open(path, "rb").read()
    except FileNotFoundError:
        sys.exit(f"{name}: {path} is not there. Fetch the corpora it is in "
                 "(make cff-fonts) and run again.")
    font = TTFont(io.BytesIO(data))
    n = len(font.getGlyphOrder())
    out.append(f"face {name} {hashlib.sha256(data).hexdigest()}")
    for label, gids in cases(name, font, data, n):
        kept, g, l = survivors(data, gids)
        out.append(f"case {label}")
        out.append("glyphs " + " ".join(map(str, kept)))
        out.append("global " + " ".join(map(str, g)))
        out.append("local " + " ".join(map(str, l)))

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/cffsubrs.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face and each set of glyphs asked for: the glyphs fontTools'\n")
    w.write("# subsetter keeps (the set, .notdef and any seac's components), and the\n")
    w.write("# original indices of the global and local subroutines it keeps with them.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# fonttools {ft.version}\n")
    for line in out:
        w.write(line + "\n")
