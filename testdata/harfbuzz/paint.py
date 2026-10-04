# Asks HarfBuzz to paint every glyph of the colour faces, as
# hb_font_paint_glyph paints them, and writes each callback it makes, so that
# shape/paint_test.go can hold Face.PaintGlyph to them.
#
#   make hbpaint
#
# The faces are named on the command line as NAME=PATH, each followed by what
# it is painted under:
#
#   NAME=PATH            at its default, in palette 0
#   ...@wght=W           at a weight, for the variable faces
#   ...:palette=P        in another palette, or one past the font's last
#   ...:ppem=N           with the font's size set, which picks the bitmap strike
#   ...:glyphs=A,B       only those glyphs, for a corpus face too large to
#                        write whole
#   ...:overrides=I/RRGGBBAA+...  with palette entry I overridden by a colour,
#                        as CSS font-palette's override-colors asks, through
#                        HarfBuzz's custom palette colour callback
#
# An SVG glyph's image is handed over with no extents, and is written with
# "none" in their place.
# Every glyph is painted with a foreground of 0x33669 9 at an alpha of 0xCC, not
# opaque, so that what painting does to the foreground's alpha shows.
#
# What is written is the callbacks less the identity transforms. HarfBuzz
# pushes the font's own transform around a glyph and its inverse around each
# outline a fill is clipped to; at the font's own scale, which is the scale
# painting is in here, both are the identity, and a caller painting in font
# units has nothing to do with them. One is dropped with the pop that matches
# it.
#
# uharfbuzz 0.56.2 cannot pass an image to Python: its wrapper hands
# hb_tag_to_string a null buffer and crashes. The bitmap faces are painted
# through the same HarfBuzz, called with ctypes from the library uharfbuzz
# carries, with callbacks for the image and for the outline a glyph without
# one falls back to.
import ctypes
import hashlib
import sys

from oracle import harfbuzz

hb = harfbuzz()

out_path = sys.argv[1]
FOREGROUND = (0x33, 0x66, 0x99, 0xCC)


def num(v):
    return repr(float(v))


def colour(c, foreground):
    return f"{c.red} {c.green} {c.blue} {c.alpha} {int(bool(foreground))}"


def stops(line):
    return " | ".join(f"{num(s.offset)} {colour(s.color, s.is_foreground)}"
                      for s in line.color_stops)


EXTEND = {0: "pad", 1: "repeat", 2: "reflect"}


class Recorder:
    """The callbacks, with each identity transform and its pop left out."""

    def __init__(self):
        self.lines = []
        self.pushed = []

    def push_transform(self, xx, yx, xy, yy, x0, y0, _):
        identity = (xx, yx, xy, yy, x0, y0) == (1, 0, 0, 1, 0, 0)
        self.pushed.append(not identity)
        if not identity:
            self.lines.append("T " + " ".join(num(v) for v in (xx, yx, xy, yy, x0, y0)))

    def pop_transform(self, _):
        if self.pushed.pop():
            self.lines.append("t")

    def clip_glyph(self, gid, _):
        self.lines.append(f"CG {gid}")

    def clip_rect(self, xmin, ymin, xmax, ymax, _):
        self.lines.append("CR " + " ".join(num(v) for v in (xmin, ymin, xmax, ymax)))

    def pop_clip(self, _):
        self.lines.append("c")

    def color(self, c, foreground, _):
        self.lines.append("S " + colour(c, foreground))

    def linear(self, line, x0, y0, x1, y1, x2, y2, _):
        pts = " ".join(num(v) for v in (x0, y0, x1, y1, x2, y2))
        self.lines.append(f"L {EXTEND[int(line.extend)]} {pts} | {stops(line)}")

    def radial(self, line, x0, y0, r0, x1, y1, r1, _):
        pts = " ".join(num(v) for v in (x0, y0, r0, x1, y1, r1))
        self.lines.append(f"R {EXTEND[int(line.extend)]} {pts} | {stops(line)}")

    def sweep(self, line, cx, cy, start, end, _):
        pts = " ".join(num(v) for v in (cx, cy, start, end))
        self.lines.append(f"W {EXTEND[int(line.extend)]} {pts} | {stops(line)}")

    def push_group(self, _):
        self.lines.append("G")

    def pop_group(self, mode, _):
        self.lines.append(f"g {int(mode)}")


def funcs_for(rec, overrides):
    f = hb.PaintFuncs()
    if overrides:
        f.set_custom_palette_color_func(lambda index, _: overrides.get(index))
    f.set_push_transform_func(rec.push_transform)
    f.set_pop_transform_func(rec.pop_transform)
    f.set_push_clip_glyph_func(rec.clip_glyph)
    f.set_push_clip_rectangle_func(rec.clip_rect)
    f.set_pop_clip_func(rec.pop_clip)
    f.set_color_func(rec.color)
    f.set_linear_gradient_func(rec.linear)
    f.set_radial_gradient_func(rec.radial)
    f.set_sweep_gradient_func(rec.sweep)
    f.set_push_group_func(rec.push_group)
    f.set_pop_group_func(rec.pop_group)
    return f


def paint_colr(data, weight, palette, out, only=None, overrides=None):
    face = hb.Face(data)
    font = hb.Font(face)
    if weight is not None:
        font.set_variations({"wght": weight})
    fg = hb.Color(*FOREGROUND)
    for gid in only or range(face.glyph_count):
        rec = Recorder()
        font.paint_glyph(gid, funcs_for(rec, overrides), None, palette, fg)
        assert not rec.pushed, "a transform left pushed"
        out.append(f"G {gid}")
        out.extend(rec.lines)


# The C side, for the bitmap faces: HarfBuzz's own types, as hb.h states them.
lib = ctypes.CDLL(hb._harfbuzz.__file__)


class Extents(ctypes.Structure):
    _fields_ = [("x_bearing", ctypes.c_int32), ("y_bearing", ctypes.c_int32),
                ("width", ctypes.c_int32), ("height", ctypes.c_int32)]


VP = ctypes.c_void_p
IMAGE = ctypes.CFUNCTYPE(ctypes.c_int, VP, VP, VP, ctypes.c_uint, ctypes.c_uint,
                         ctypes.c_uint32, ctypes.c_float, ctypes.POINTER(Extents), VP)
CLIP_GLYPH = ctypes.CFUNCTYPE(None, VP, VP, ctypes.c_uint32, VP, VP)
COLOR = ctypes.CFUNCTYPE(None, VP, VP, ctypes.c_int, ctypes.c_uint32, VP)
POP = ctypes.CFUNCTYPE(None, VP, VP, VP)
lib.hb_blob_create.restype = VP
lib.hb_blob_create.argtypes = [ctypes.c_char_p, ctypes.c_uint, ctypes.c_int, VP, VP]
lib.hb_face_create.restype = VP
lib.hb_face_create.argtypes = [VP, ctypes.c_uint]
lib.hb_font_create.restype = VP
lib.hb_font_create.argtypes = [VP]
lib.hb_font_set_ppem.argtypes = [VP, ctypes.c_uint, ctypes.c_uint]
lib.hb_face_get_glyph_count.restype = ctypes.c_uint
lib.hb_face_get_glyph_count.argtypes = [VP]
lib.hb_paint_funcs_create.restype = VP
for name in ("hb_paint_funcs_set_image_func", "hb_paint_funcs_set_push_clip_glyph_func",
             "hb_paint_funcs_set_color_func", "hb_paint_funcs_set_pop_clip_func"):
    getattr(lib, name).argtypes = [VP, VP, VP, VP]
lib.hb_font_paint_glyph.argtypes = [VP, ctypes.c_uint32, VP, VP, ctypes.c_uint, ctypes.c_uint32]
lib.hb_blob_get_data.restype = ctypes.POINTER(ctypes.c_char)
lib.hb_blob_get_data.argtypes = [VP, ctypes.POINTER(ctypes.c_uint)]
for name in ("hb_font_destroy", "hb_face_destroy", "hb_blob_destroy", "hb_paint_funcs_destroy"):
    getattr(lib, name).argtypes = [VP]

HB_MEMORY_MODE_READONLY = 1


def hb_color(r, g, b, a):
    return b << 24 | g << 16 | r << 8 | a


def paint_bitmaps(data, ppem, out, only=None):
    lines = []

    def image(_f, _d, blob, width, height, fmt, slant, ext, _u):
        n = ctypes.c_uint()
        ptr = lib.hb_blob_get_data(blob, ctypes.byref(n))
        png = ctypes.string_at(ptr, n.value)
        tag = fmt.to_bytes(4, "big").decode("latin-1")
        extents = "none"
        if ext:
            e = ext.contents
            extents = f"{e.x_bearing} {e.y_bearing} {e.width} {e.height}"
        lines.append(f"I {width} {height} {tag.strip()} {num(slant)} {extents} "
                     f"{len(png)} {hashlib.sha256(png).hexdigest()[:16]}")
        return 1

    def clip_glyph(_f, _d, gid, _font, _u):
        lines.append(f"CG {gid}")

    def color(_f, _d, foreground, c, _u):
        r, g, b, a = (c >> 8) & 0xFF, (c >> 16) & 0xFF, (c >> 24) & 0xFF, c & 0xFF
        lines.append(f"S {r} {g} {b} {a} {int(bool(foreground))}")

    def pop_clip(_f, _d, _u):
        lines.append("c")

    keep = [IMAGE(image), CLIP_GLYPH(clip_glyph), COLOR(color), POP(pop_clip)]
    funcs = lib.hb_paint_funcs_create()
    lib.hb_paint_funcs_set_image_func(funcs, ctypes.cast(keep[0], VP), None, None)
    lib.hb_paint_funcs_set_push_clip_glyph_func(funcs, ctypes.cast(keep[1], VP), None, None)
    lib.hb_paint_funcs_set_color_func(funcs, ctypes.cast(keep[2], VP), None, None)
    lib.hb_paint_funcs_set_pop_clip_func(funcs, ctypes.cast(keep[3], VP), None, None)
    blob = lib.hb_blob_create(data, len(data), HB_MEMORY_MODE_READONLY, None, None)
    face = lib.hb_face_create(blob, 0)
    font = lib.hb_font_create(face)
    lib.hb_font_set_ppem(font, ppem, ppem)
    for gid in only or range(lib.hb_face_get_glyph_count(face)):
        lines.clear()
        lib.hb_font_paint_glyph(font, gid, funcs, None, 0, hb_color(*FOREGROUND))
        out.append(f"G {gid}")
        out.extend(lines)
    lib.hb_font_destroy(font)
    lib.hb_face_destroy(face)
    lib.hb_blob_destroy(blob)
    lib.hb_paint_funcs_destroy(funcs)


out = []
for arg in sys.argv[2:]:
    name, rest = arg.split("=", 1)
    path, *opts = rest.split(":")
    weight = None
    if "@" in path:
        path, w = path.split("@")
        weight = int(w.split("=")[1])
    settings = dict(o.split("=") for o in opts)
    palette, ppem = int(settings.get("palette", 0)), int(settings.get("ppem", 0))
    only = [int(g) for g in settings["glyphs"].split(",")] if "glyphs" in settings else None
    spec = settings.get("overrides", "-")
    overrides = {}
    if spec != "-":
        for o in spec.split("+"):
            index, rgba = o.split("/")
            overrides[int(index)] = hb.Color(*bytes.fromhex(rgba))
    data = open(path, "rb").read()
    label = name + (f"@wght={weight}" if weight is not None else "")
    out.append(f"face {label} palette {palette} ppem {ppem} overrides {spec} "
               f"{hashlib.sha256(data).hexdigest()}")
    if "bitmap" in settings:
        paint_bitmaps(data, ppem, out, only)
    else:
        paint_colr(data, weight, palette, out, only, overrides)

with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/paint.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# For each face, painted at a weight, in a palette, at a ppem and with\n")
    w.write("# palette entries overridden (index/RRGGBBAA, or - for none), each\n")
    w.write("# glyph's painting as HarfBuzz's callbacks make it, less the identity\n")
    w.write("# transforms: T and t push and pop a transform (xx yx xy yy x0 y0); CG and\n")
    w.write("# CR push a clip to a glyph's outline and to a box, and c pops one; G and\n")
    w.write("# g push a group and pop it in a composite mode; S is a solid fill (red,\n")
    w.write("# green, blue, alpha, whether the foreground); L, R and W are a linear,\n")
    w.write("# radial and sweep gradient, its extend, its geometry and its stops (offset\n")
    w.write("# and colour); I is an image: its width and height in pixels, format,\n")
    w.write("# slant, extents in font units (none for an SVG document, which HarfBuzz\n")
    w.write("# hands over with none), length and the start of its SHA-256.\n")
    w.write("# A face painted for some of its glyphs only names each one it paints.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    for line in out:
        w.write(line + "\n")
