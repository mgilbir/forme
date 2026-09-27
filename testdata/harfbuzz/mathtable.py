# Asks HarfBuzz what each face's MATH table says — every constant, every
# glyph's italics correction, top accent attachment, extended shape, kerning,
# size variants and assemblies — and writes the answers, so that
# shape/math_test.go can hold shape/math.go to them.
#
#   make hbmath
#
# The companion of cffink.py, and the same bargain: the output is checked in,
# so running the test needs a Go toolchain and regenerating it needs Python and
# the pinned uharfbuzz and fontTools.
#
# fontTools is asked too, and the two must agree before anything is written:
# HarfBuzz is the answer a browser gets, and fontTools reads the table's
# records as they are stored, so a value that passes both has been read twice
# by readers that share no code. fontTools is also what says whether a glyph
# is covered at all, which HarfBuzz does not: it answers a top accent
# attachment for every glyph, half the advance where the font states none,
# and nought for an italics correction the font does not state. Only what the
# font states is written; shape/math_test.go asks about every glyph of every
# face and requires "not stated" for every one not written here.
#
# The faces are named on the command line as NAME=PATH:
#
#   MathTable.ttf      built by mathtable_fixture.py, every part of the table
#   the rest           the math fonts in the corpora: some of the suite's own
#                      test fonts from fonts/math (WOFF, unwrapped by fontTools
#                      before HarfBuzz reads them), and Noto Sans Math and
#                      STIX Two Math from the Google Fonts library
#
# A face whose file is not there stops the run rather than being left out: an
# expectation file that quietly covers fewer faces is one the Go test would
# pass on for less.
import hashlib
import io
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
fonttools()
from fontTools.ttLib import TTFont  # noqa: E402

out_path = sys.argv[1]
faces = [a.split("=", 1) for a in sys.argv[2:]]


def sfnt(data):
    """The font as an sfnt HarfBuzz reads: a WOFF unwrapped, its tables as
    they were."""
    if data[:4] != b"wOFF":
        return data
    f = TTFont(io.BytesIO(data), lazy=True)
    f.flavor = None
    buf = io.BytesIO()
    f.save(buf, reorderTables=False)
    return buf.getvalue()


def disagree(name, what, want, got):
    sys.exit(f"{name}: {what}: HarfBuzz says {got!r} and fontTools {want!r}; "
             "the oracle does not agree with itself, so nothing is written")


def covered(coverage, records):
    """{glyph name: coverage index} for the glyphs a subtable states a record
    for — its coverage, less any index past its records."""
    if coverage is None:
        return {}
    return {g: i for i, g in enumerate(coverage.glyphs) if i < len(records)}


CORNERS = ["TopRight", "TopLeft", "BottomRight", "BottomLeft"]

out = []
for name, path in faces:
    try:
        data = open(path, "rb").read()
    except FileNotFoundError:
        sys.exit(f"{name}: {path} is not there. Fetch the corpora it is in "
                 "(make wpt googlefonts) and run again.")
    raw = sfnt(data)
    face = hb.Face(raw)
    font = hb.Font(face)
    tt = TTFont(io.BytesIO(raw))
    gid = {n: i for i, n in enumerate(tt.getGlyphOrder())}
    n = face.glyph_count
    out.append(f"face {name} {hashlib.sha256(data).hexdigest()}")
    if "MATH" not in tt:
        sys.exit(f"{name}: has no MATH table")
    if not face.has_math_data:
        sys.exit(f"{name}: HarfBuzz reads no MATH data from it")
    table = tt["MATH"].table

    # The constants: all fifty-six, or none where the subtable is absent.
    mc = table.MathConstants
    if mc is None:
        out.append("C none")
    else:
        values = []
        for i, c in enumerate(hb.OTMathConstant):
            got = font.get_math_constant(c)
            field = getattr(mc, "".join(w.capitalize() for w in c.name.split("_")))
            want = field.Value if hasattr(field, "Value") else field
            if want != got:
                disagree(name, f"constant {c.name}", want, got)
            values.append(str(got))
        out.append("C " + " ".join(values))

    info = table.MathGlyphInfo
    italics = info.MathItalicsCorrectionInfo if info else None
    accents = info.MathTopAccentAttachment if info else None
    ic = covered(italics.Coverage, italics.ItalicsCorrection) if italics else {}
    ta = covered(accents.TopAccentCoverage, accents.TopAccentAttachment) if accents else {}
    ext = set(info.ExtendedShapeCoverage.glyphs) if info and info.ExtendedShapeCoverage else set()
    kinfo = info.MathKernInfo if info else None
    kerns = covered(kinfo.MathKernCoverage, kinfo.MathKernInfoRecords) if kinfo else {}

    mv = table.MathVariants
    overlap_h = font.get_math_min_connector_overlap("ltr")
    overlap_v = font.get_math_min_connector_overlap("btt")
    want = mv.MinConnectorOverlap if mv else 0
    if overlap_h != want or overlap_v != want:
        disagree(name, "min connector overlap", want, (overlap_h, overlap_v))
    out.append(f"O {want}")

    def constructions(cov, recs):
        return covered(cov, recs) if mv and cov is not None else {}

    vert = constructions(mv.VertGlyphCoverage if mv else None,
                         mv.VertGlyphConstruction if mv else [])
    horiz = constructions(mv.HorizGlyphCoverage if mv else None,
                          mv.HorizGlyphConstruction if mv else [])

    for g, glyph in enumerate(tt.getGlyphOrder()):
        if g >= n:
            break
        if glyph in ic:
            want = italics.ItalicsCorrection[ic[glyph]].Value
            got = font.get_math_glyph_italics_correction(g)
            if want != got:
                disagree(name, f"italics correction of {glyph}", want, got)
            out.append(f"I {g} {got}")
        elif font.get_math_glyph_italics_correction(g) != 0:
            disagree(name, f"italics correction of uncovered {glyph}", 0,
                     font.get_math_glyph_italics_correction(g))
        if glyph in ta:
            want = accents.TopAccentAttachment[ta[glyph]].Value
            got = font.get_math_glyph_top_accent_attachment(g)
            if want != got:
                disagree(name, f"top accent attachment of {glyph}", want, got)
            out.append(f"A {g} {got}")
        if (glyph in ext) != face.is_glyph_extended_math_shape(g):
            disagree(name, f"extended shape {glyph}", glyph in ext,
                     face.is_glyph_extended_math_shape(g))
        if glyph in ext:
            out.append(f"X {g}")
        if glyph in kerns:
            rec = kinfo.MathKernInfoRecords[kerns[glyph]]
            for corner, side in enumerate(CORNERS):
                k = getattr(rec, side + "MathKern")
                if k is None:
                    continue
                heights = [h.Value for h in k.CorrectionHeight]
                probes = sorted({-100000, 100000} | {h + d for h in heights for d in (-1, 0, 1)})
                for h in probes:
                    got = font.get_math_glyph_kerning(g, corner, h)
                    # The OpenType step: kern i where heights[i-1] <= h < heights[i].
                    i = next((j for j, v in enumerate(heights) if h < v), len(heights))
                    want = k.KernValue[i].Value
                    if want != got:
                        disagree(name, f"kern of {glyph} at {side} {h}", want, got)
                    out.append(f"K {g} {corner} {h} {got}")
        for axis, cons, direction in (("V", vert, "btt"), ("H", horiz, "ltr")):
            if glyph not in cons:
                continue
            recs = mv.VertGlyphConstruction if axis == "V" else mv.HorizGlyphConstruction
            c = recs[cons[glyph]]
            variants = font.get_math_glyph_variants(g, direction)
            want = [(gid[v.VariantGlyph], v.AdvanceMeasurement) for v in c.MathGlyphVariantRecord]
            got = [(v.glyph, v.advance) for v in variants]
            if want != got:
                disagree(name, f"{axis} variants of {glyph}", want, got)
            out.append(f"{axis} {g} " + " ".join(f"{v}:{a}" for v, a in got))
            parts, italic = font.get_math_glyph_assembly(g, direction)
            got = [(p.glyph, p.start_connector_length, p.end_connector_length,
                    p.full_advance, int(p.flags)) for p in parts]
            if c.GlyphAssembly is None:
                if got:
                    disagree(name, f"{axis} assembly of {glyph}", [], got)
                continue
            a = c.GlyphAssembly
            want = [(gid[p.glyph], p.StartConnectorLength, p.EndConnectorLength,
                     p.FullAdvance, p.PartFlags) for p in a.PartRecords]
            if want != got or a.ItalicsCorrection.Value != italic:
                disagree(name, f"{axis} assembly of {glyph}",
                         (want, a.ItalicsCorrection.Value), (got, italic))
            out.append(f"{axis}A {g} {italic} " +
                       " ".join(",".join(str(v) for v in p) for p in got))

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/mathtable.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face: C is its fifty-six MathConstants in the table's order,\n")
    w.write("# or none; O its minimum connector overlap; I and A a glyph's italics\n")
    w.write("# correction and top accent attachment, where the font states one; X a\n")
    w.write("# glyph the font marks as an extended shape; K glyph, corner, height and\n")
    w.write("# the kern HarfBuzz answers there; V and H a glyph's vertical and\n")
    w.write("# horizontal size variants as glyph:advance; VA and HA its assembly, as\n")
    w.write("# italics correction and then glyph,start,end,full advance,flags for each\n")
    w.write("# part. Everything is in font units.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    for line in out:
        w.write(line + "\n")
