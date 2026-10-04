# Positions strings with HarfBuzz in faces with AAT's kerx and trak, and
# writes what it produces, so that shape/kerx_test.go can hold shape/kerx.go
# and shape/trak.go to it.
#
#   make hbaatpos
#
# The faces are those aatpos_fixture.py builds (see there for what each
# states), and TRAK.ttf, the face of HarfBuzz's own aat-trak tests, kept in
# aat-inhouse/ with its cases, which this also checks HarfBuzz still sets as
# they say before writing anything.
#
# HarfBuzz's own font functions read no points of a glyph's outline, and kerx
# format 4 attaches nothing by them; CoreText reads them, and so does HarfBuzz
# over FreeType, whose hb_ft_get_glyph_contour_point hands it the points of
# the outline FreeType loads. shape/kerx.go does as they do. So the faces are
# set here by the pinned HarfBuzz, called with ctypes from the library
# uharfbuzz carries, on a font whose one function of its own is that one: each
# point the glyph's glyf states, a composite's components resolved by
# fontTools, moved so that the left phantom point is the origin, as FreeType's
# TrueType loader moves the outline. Everything else is HarfBuzz's own.
#
# Each case is a face, a string, the size it is set at in points (0 for none,
# which is HarfBuzz's and CoreText's 12), whether kerning is on, and the
# glyphs HarfBuzz sets: index, cluster (a byte offset into the UTF-8 string),
# x advance, x offset and y offset, in font units.
import ctypes
import hashlib
import os
import sys

from oracle import fonttools, harfbuzz

hb = harfbuzz()
fonttools()
from fontTools.misc.roundTools import otRound  # noqa: E402
from fontTools.ttLib import TTFont  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
out_path = sys.argv[1]

ACUTE = "́"
STRINGS = {
    "KerxPairs.ttf": ["AV", "VA", "To", "A" + ACUTE + "V", "AVTo", "BC", "CB", "BB", "CC", "DA",
                      "VT", "oT", "BBB", "BBBC", "AVBBo", "ToVA"],
    "KerxMachines.ttf": ["AV", "VA", "AAV", "ATV", "AVAV", "o", "oo", "oBo", "Bo", "A" + ACUTE,
                         "D" + ACUTE, "C" + ACUTE, "A" + ACUTE + "V", "oA" + ACUTE, "CD" + ACUTE,
                         "C" + ACUTE + ACUTE],
    "KerxPoints.ttf": ["\uE000" + ACUTE, "\uE001" + ACUTE, "D" + ACUTE, "D" + ACUTE + ACUTE,
                       "\uE000" + ACUTE + ACUTE, "\uE001D" + ACUTE, "A" + ACUTE],
    "KerxPlanGSUBGPOS.ttf": ["AV", "A" + ACUTE + "V"],
    "KerxPlanGPOS.ttf": ["AV", "A" + ACUTE + "V"],
    "KerxPlanNoKern.ttf": ["AV", "A" + ACUTE, "A" + ACUTE + "V"],
    "KerxPlanLegacyKern.ttf": ["AV", "A" + ACUTE + "V"],
    "TrakCases.ttf": ["AA", "A" + ACUTE + "A", "\U0001F600‍❤", "\U0001F1EF\U0001F1F5",
                      "A\U0001F1EF\U0001F1F5\U0001F1EF"],
    "TrakNoSTAT.ttf": ["AA"],
}
SIZES = {"TrakCases.ttf": [0, 6, 9, 10.5, 12, 24, 100], "TrakNoSTAT.ttf": [0, 6]}


def cases():
    for name, strings in STRINGS.items():
        for s in strings:
            for size in SIZES.get(name, [0]):
                for kern in (True, False):
                    yield name, os.path.join(HERE, "fonts", name), s, size, kern, None
    # HarfBuzz's own tracking cases: the face, a size, and what it expects.
    for line in open(os.path.join(HERE, "aat-inhouse", "tests", "aat-trak.tests"), encoding="utf-8"):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        font, options, codepoints, expected = line.split(";")
        size = 0.0
        if options:
            size = float(options.split("=")[1])
        text = "".join(chr(int(c[2:], 16)) for c in codepoints.split(","))
        path = os.path.join(HERE, "aat-inhouse", "fonts", os.path.basename(font))
        yield os.path.basename(font), path, text, size, True, expected


def names(font, buf):
    """hb-shape's default notation: name=cluster+advance, with offsets."""
    out = []
    for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
        g = font.glyph_to_string(info.codepoint) + f"={info.cluster}"
        if pos.x_offset or pos.y_offset:
            g += f"@{pos.x_offset},{pos.y_offset}"
        g += f"+{pos.x_advance}"
        out.append(g)
    return "[" + "|".join(out) + "]"


def contour_points(path):
    """Each glyph's points as FreeType loads them: glyf's, a composite's
    components resolved, rounded, and moved by the left phantom point,
    which is the glyph's xMin less its side bearing."""
    f = TTFont(path)
    if "glyf" not in f:
        return {}
    glyf, hmtx = f["glyf"], f["hmtx"]
    out = {}
    for gid, name in enumerate(f.getGlyphOrder()):
        g = glyf[name]
        coords, _, _ = g.getCoordinates(glyf)
        if not len(coords):
            out[gid] = []
            continue
        shift = g.xMin - hmtx[name][1]
        out[gid] = [(otRound(x) - shift, otRound(y)) for x, y in coords]
    return out


lib = ctypes.CDLL(hb._harfbuzz.__file__)
VP = ctypes.c_void_p


class Info(ctypes.Structure):
    _fields_ = [("codepoint", ctypes.c_uint32), ("mask", ctypes.c_uint32),
                ("cluster", ctypes.c_uint32), ("var1", ctypes.c_uint32), ("var2", ctypes.c_uint32)]


class Pos(ctypes.Structure):
    _fields_ = [("x_advance", ctypes.c_int32), ("y_advance", ctypes.c_int32),
                ("x_offset", ctypes.c_int32), ("y_offset", ctypes.c_int32), ("var", ctypes.c_uint32)]


class Feature(ctypes.Structure):
    _fields_ = [("tag", ctypes.c_uint32), ("value", ctypes.c_uint32),
                ("start", ctypes.c_uint), ("end", ctypes.c_uint)]


CONTOUR = ctypes.CFUNCTYPE(ctypes.c_int, VP, VP, ctypes.c_uint32, ctypes.c_uint,
                           ctypes.POINTER(ctypes.c_int32), ctypes.POINTER(ctypes.c_int32), VP)
for fn, res, args in [
    ("hb_blob_create", VP, [ctypes.c_char_p, ctypes.c_uint, ctypes.c_int, VP, VP]),
    ("hb_face_create", VP, [VP, ctypes.c_uint]),
    ("hb_font_create", VP, [VP]),
    ("hb_font_create_sub_font", VP, [VP]),
    ("hb_font_funcs_create", VP, []),
    ("hb_font_funcs_set_glyph_contour_point_func", None, [VP, CONTOUR, VP, VP]),
    ("hb_font_set_funcs", None, [VP, VP, VP, VP]),
    ("hb_font_set_ptem", None, [VP, ctypes.c_float]),
    ("hb_buffer_create", VP, []),
    ("hb_buffer_add_utf8", None, [VP, ctypes.c_char_p, ctypes.c_int, ctypes.c_uint, ctypes.c_int]),
    ("hb_buffer_guess_segment_properties", None, [VP]),
    ("hb_feature_from_string", ctypes.c_int, [ctypes.c_char_p, ctypes.c_int, ctypes.POINTER(Feature)]),
    ("hb_shape", None, [VP, VP, ctypes.POINTER(Feature), ctypes.c_uint]),
    ("hb_buffer_get_glyph_infos", ctypes.POINTER(Info), [VP, ctypes.POINTER(ctypes.c_uint)]),
    ("hb_buffer_get_glyph_positions", ctypes.POINTER(Pos), [VP, ctypes.POINTER(ctypes.c_uint)]),
    ("hb_buffer_destroy", None, [VP]),
    ("hb_font_destroy", None, [VP]),
    ("hb_font_funcs_destroy", None, [VP]),
    ("hb_face_destroy", None, [VP]),
    ("hb_blob_destroy", None, [VP]),
]:
    getattr(lib, fn).restype = res
    getattr(lib, fn).argtypes = args


def shape_with_points(data, points, text, size, kern):
    """The glyphs HarfBuzz sets a string as, on a font whose contour points
    are points': index, cluster (a byte offset, as hb_buffer_add_utf8 makes
    it), x advance, x offset and y offset."""
    def contour(_font, _data, gid, i, x, y, _user):
        p = points.get(gid, [])
        if i >= len(p):
            return 0
        x[0], y[0] = p[i]
        return 1

    keep = CONTOUR(contour)
    blob = lib.hb_blob_create(data, len(data), 1, None, None)
    face = lib.hb_face_create(blob, 0)
    parent = lib.hb_font_create(face)
    font = lib.hb_font_create_sub_font(parent)
    funcs = lib.hb_font_funcs_create()
    lib.hb_font_funcs_set_glyph_contour_point_func(funcs, keep, None, None)
    lib.hb_font_set_funcs(font, funcs, None, None)
    if size:
        lib.hb_font_set_ptem(font, size)
    buf = lib.hb_buffer_create()
    utf8 = text.encode("utf-8")
    lib.hb_buffer_add_utf8(buf, utf8, len(utf8), 0, len(utf8))
    lib.hb_buffer_guess_segment_properties(buf)
    feats = (Feature * 1)()
    n = 0
    if not kern:
        lib.hb_feature_from_string(b"-kern", -1, feats)
        n = 1
    lib.hb_shape(font, buf, feats, n)
    count = ctypes.c_uint()
    infos = lib.hb_buffer_get_glyph_infos(buf, ctypes.byref(count))
    pos = lib.hb_buffer_get_glyph_positions(buf, ctypes.byref(count))
    out = [(infos[i].codepoint, infos[i].cluster, pos[i].x_advance, pos[i].x_offset, pos[i].y_offset)
           for i in range(count.value)]
    for fn, obj in (("hb_buffer_destroy", buf), ("hb_font_destroy", font), ("hb_font_destroy", parent),
                    ("hb_font_funcs_destroy", funcs), ("hb_face_destroy", face), ("hb_blob_destroy", blob)):
        getattr(lib, fn)(obj)
    return out


points_of = {}
out = []
for name, path, text, size, kern, expected in cases():
    data = open(path, "rb").read()
    if expected is not None:
        face = hb.Face(data)
        font = hb.Font(face)
        font.scale = (face.upem, face.upem)
        if size:
            font.ptem = size
        buf = hb.Buffer()
        buf.add_str(text)
        buf.guess_segment_properties()
        hb.shape(font, buf)
        if names(font, buf) != expected:
            sys.exit(f"HarfBuzz sets {text!r} in {name} at {size} as {names(font, buf)}, "
                     f"and its own test expects {expected}")
    if path not in points_of:
        points_of[path] = contour_points(path)
    shaped = shape_with_points(data, points_of[path], text, size, kern)
    glyphs = " ".join(",".join(str(v) for v in g) for g in shaped)
    codes = ",".join(f"{ord(c):04X}" for c in text)
    out.append(f"{name} {hashlib.sha256(data).hexdigest()} {codes} {size:g} {int(kern)} {glyphs}")

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/aatpos.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# Each case: the face, its SHA-256, the code points, the size in points\n")
    w.write("# (0 for none), whether kerning is on, and the glyphs HarfBuzz sets —\n")
    w.write("# index, cluster (a byte offset into the UTF-8 string), x advance,\n")
    w.write("# x offset and y offset, in font units.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# cases {len(out)}\n")
    for line in out:
        w.write(line + "\n")
