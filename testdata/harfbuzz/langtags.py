# Asks HarfBuzz which OpenType language system tags each of a list of BCP 47
# language tags selects, and writes the answers.
#
# The output is checked in and is what shape/language_test.go compares
# shape.openTypeLanguages against. Regenerating it needs Python and uharfbuzz;
# running the test needs neither.
#
#   python3 testdata/harfbuzz/langtags.py testdata/harfbuzz-langtags/hb-ot-tag-table.hh \
#       testdata/harfbuzz/langtags.expected.txt
#
# uharfbuzz does not wrap hb_ot_tags_from_script_and_language, so it is called
# through ctypes in the library uharfbuzz carries — the same HarfBuzz that
# shapes the other oracles here. The header records its version, which has to
# be the release package shape's table was generated from: the two are the
# same join of the same registries only then.
#
# The tags asked about are read out of the header the table was generated
# from — every primary subtag either of its tables names, every one it blocks,
# every rule of hb_ot_tags_from_complex_language turned into tags that should
# and should not match it — and then varied: in capitals, with "_" for "-",
# with a region, a script, an extension or a private-use part after them, and
# cut short. A tag list written by hand would test what its writer thought the
# rules were.
import ctypes
import glob
import os
import random
import re
import sys

import uharfbuzz as hb

header_path, out_path = sys.argv[1], sys.argv[2]
header = open(header_path, encoding="utf-8").read()

lib = ctypes.CDLL(glob.glob(os.path.join(os.path.dirname(hb.__file__), "_harfbuzz*.so"))[0])
lib.hb_language_from_string.restype = ctypes.c_void_p
lib.hb_language_from_string.argtypes = [ctypes.c_char_p, ctypes.c_int]
lib.hb_ot_tags_from_script_and_language.argtypes = [
    ctypes.c_uint32, ctypes.c_void_p,
    ctypes.POINTER(ctypes.c_uint), ctypes.POINTER(ctypes.c_uint32),
    ctypes.POINTER(ctypes.c_uint), ctypes.POINTER(ctypes.c_uint32),
]

# HB_OT_MAX_TAGS_PER_LANGUAGE: what the shaper asks for.
MAX_TAGS = 3


def tags_of(lang):
    n = ctypes.c_uint(MAX_TAGS)
    arr = (ctypes.c_uint32 * MAX_TAGS)()
    lib.hb_ot_tags_from_script_and_language(
        0, lib.hb_language_from_string(lang.encode("latin-1"), -1),
        None, None, ctypes.byref(n), arr)
    return [arr[i] for i in range(n.value)]


def spell(tag):
    b = tag.to_bytes(4, "big")
    if all(0x20 <= c <= 0x7E for c in b) and b"|" not in b:
        return b.decode("ascii")
    return "0x%08x" % tag


def unquote(t):
    return "".join(re.findall(r"'(.)'", t)).rstrip()


primaries = set()
for m in re.finditer(r"\{HB_TAG\(([^)]*)\),\s*HB_TAG", header):
    primaries.add(unquote(m.group(1)))
for name in ("ot_languages3_blocked", "ot_languages3_multi"):
    i = header.index(name + "[] = {")
    body = header[i:header.index("\n};", i)]
    for m in re.finditer(r"HB_TAG\(('.','.','.','.')\)", body):
        primaries.add(unquote(m.group(1)))

rules = []
fn = header[header.index("hb_ot_tags_from_complex_language (const char"):]
fn = fn[:fn.index("\n}\n")]
first = None
for line in fn.split("\n"):
    m = re.match(r"^  case '(.)':$", line)
    if m:
        first = m.group(1)
        continue
    m = re.search(r'subtag_matches \(p, limit, "([^"]+)"', line)
    if m:
        rules.append(("subtag", "", m.group(1)))
        continue
    m = re.search(r'strcmp \(&lang_str\[1\], "([^"]+)"\)\)', line)
    if m and "strncmp" not in line:
        rules.append(("exact", first, m.group(1)))
        continue
    m = re.search(r'lang_matches \(&lang_str\[1\], limit, "([^"]+)"', line)
    if m:
        rules.append(("prefix", first, m.group(1)))
        continue
    m = re.search(r'strncmp \(&lang_str\[1\], "([^"]+)"', line)
    if m:
        rules.append(("start", first, m.group(1)))
        continue
    m = re.search(r'&& subtag_matches \(lang_str, limit, "([^"]+)"', line)
    if m:
        kind, f, spec = rules.pop()
        rules.append(("start", f, spec + "\0" + m.group(1)))

langs = set()
for p in sorted(primaries):
    langs.update([p, p.upper(), p + "-US", p + "_hk", p + "-Latn", p + "-x-a",
                  p + "-u-ca-x", "zh-" + p, "und-" + p, p + "-" + p])
for kind, f, spec in rules:
    if kind == "subtag":
        for pre in ("und", "el", "hy", "oc", "x", "en-x", "abcdefgh"):
            langs.update([pre + spec, pre + spec + "-a", pre + spec + "x",
                          pre + spec.upper() + "-x-hbotabc", pre + "-x" + spec])
        langs.add(spec[1:])
    elif kind == "exact":
        langs.update([f + spec, f + spec + "-a", (f + spec)[:-1]])
    elif kind == "prefix":
        langs.update([f + spec, f + spec + "-tw", f + spec + "x", f + spec + "-x-y",
                      (f + spec)[:-1], (f + spec).upper()])
    else:
        start, sub = spec.split("\0")
        langs.update([f + start + "hant" + sub, f + start + "latn" + sub + "-a",
                      f + start + sub[1:], f + start + "a" + sub + "x",
                      f + start + "x" + sub, f + start + "hans-x-y" + sub])

langs.update([
    "", "e", "en", "en-", "-en", "en--us", "a1b", "123", "x", "x-", "x-foo",
    "x-hbotmol", "x-hbot-4d4f4c20", "x-hbot-4d4f4c", "x-hbot-abcd", "x-hbotdflt",
    "x-hbot-44464c54", "en-x-hbotmar", "en-x-hbotmarathi", "en-x-hbot",
    "und-x-hbscdev2-hbotmar", "en-u-ca-gregory", "en-a-bbb-x-hbotabc",
    "zh-hant-hk-x", "zh-cmn-hant-mo", "zh-yue", "zh-yue-hk", "ar-arb", "sgn-ase",
    " en", "en us", "EN_us", "zh-Hant-TW", "zh-TW", "zh-HK", "zh-MO", "zh-Hans",
    "sr-Latn", "sr-Cyrl-RS", "tr-TR", "az-Cyrl", "ku-Arab", "mn-Mong-CN",
    "art-lojban", "art-lojbanx", "cdo-hant-hk", "cdo-hk", "i-klingon", "x-klingon",
    "zh-min", "zh-min-nan", "no-bok", "no-nyn", "el-polyton", "grc-polyton",
    "hy-arevmda", "oc-provenc", "und-fonipa", "und-fonnapa", "und-geok",
    "syr-syre", "syr-syrj", "syr-syrn", "und-syrc", "ro-md", "mo", "nv", "ms-my",
])

# And strings built at random out of the pieces tags are made of, for the
# combinations nobody listed.
rng = random.Random(20260923)
pieces = sorted(primaries)[:400] + ["hant", "hans", "latn", "cyrl", "hk", "mo",
                                    "tw", "x", "u", "a", "hbotmol", "polyton",
                                    "fonipa", "001", "419", "abc", "syre"]
for _ in range(3000):
    k = rng.randint(1, 5)
    langs.add("-".join(rng.choice(pieces) for _ in range(k)))

# Not a tag that begins with a character no tag can hold. HarfBuzz stores a
# language with each such character as a NUL, so " en" is a string that ends
# before it starts; its parser then reads on past that NUL, and the answer is
# made of whatever it found there. Package shape takes the tag to end at the
# first such character, which is what HarfBuzz does with one anywhere else in a
# tag, and says so in its own test.
def canon(c):
    return c.isascii() and (c.isalnum() or c in "-_")


langs = sorted(l for l in langs
               if "\n" not in l and "\t" not in l and "\0" not in l and (l == "" or canon(l[0])))
with open(out_path, "w", encoding="utf-8") as w:
    w.write("# Generated by testdata/harfbuzz/langtags.py. DO NOT EDIT.\n")
    w.write("#\n")
    w.write("# The OpenType language system tags HarfBuzz's\n")
    w.write("# hb_ot_tags_from_script_and_language gives each BCP 47 tag, at most\n")
    w.write(f"# {MAX_TAGS}, in order: the tag, a tab, and the tags separated by '|'. A\n")
    w.write("# tag that is not printable is written as 0x and eight hex digits.\n")
    w.write(f"# harfbuzz {hb.version_string()}\n")
    w.write(f"# uharfbuzz {hb.__version__}\n")
    w.write(f"# cases {len(langs)}\n")
    for l in langs:
        w.write(l + "\t" + "|".join(spell(t) for t in tags_of(l)) + "\n")
print(f"{len(langs)} cases -> {out_path}", file=sys.stderr)
