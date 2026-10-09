package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// Benchmarks for laying out whole documents in real faces, one script at a
// time and then mixed, which is where the coverage check that decides a run's
// face is asked once per box and then once per grapheme cluster.
//
// They need the Noto library and skip without it.

// benchFontSet is the fallback library with four of its faces also answering to
// their family names, so a document can ask for Noto by name the way a caller's
// own document would.
func benchFontSet(b *testing.B) FontSet {
	b.Helper()
	dir, _ := fonttest.NotoRoot()
	if !fonttest.NotoPresent(dir) {
		b.Skip("the Noto library is not in place; see NOTO_FONTS")
	}
	named := map[string]*shape.Face{}
	for family, file := range map[string]string{
		"noto sans":            "NotoSans-Regular.ttf",
		"noto sans devanagari": "NotoSansDevanagari-Regular.ttf",
		"noto sans arabic":     "NotoSansArabic-Regular.ttf",
		"noto sans jp":         "NotoSansJP-VF.ttf",
	} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			b.Fatal(err)
		}
		face, err := loadSuiteFace(data)
		if err != nil {
			b.Fatal(err)
		}
		named[family] = face
	}
	return suiteFonts{standard: StandardFonts(), fallback: notoFaces(), named: named}
}

func benchDocument(family, dir string, sentences []string, paragraphs int) string {
	var sb strings.Builder
	sb.WriteString(`<html><head><style>body { font-family: ` + family + `; font-size: 11pt }</style></head><body`)
	if dir != "" {
		sb.WriteString(` dir="` + dir + `"`)
	}
	sb.WriteString(`>`)
	for p := 0; p < paragraphs; p++ {
		sb.WriteString("<p>")
		for s := 0; s < 6; s++ {
			sb.WriteString(sentences[(p*7+s)%len(sentences)])
			sb.WriteString(" ")
		}
		sb.WriteString("</p>")
	}
	sb.WriteString("</body></html>")
	return sb.String()
}

var benchDocuments = []struct {
	name, family, dir string
	sentences         []string
}{
	{"latin", `"Noto Sans", sans-serif`, "", []string{
		"The quick brown fox jumps over the lazy dog, and the dog does not mind.",
		"Typography is the craft of arranging type to make written language legible.",
		"A paragraph of ordinary English, with commas, full stops and the occasional “quotation”.",
		"Office officials affirmed the fjord’s flora — waffles, soufflés and naïve café décor.",
		"Numbers like 1,234.56 and 7/8 appear in reports, invoices and tables of figures.",
	}},
	{"devanagari", `"Noto Sans Devanagari"`, "", []string{
		"भारत एक विशाल देश है जिसमें अनेक भाषाएँ बोली जाती हैं।",
		"हिन्दी देवनागरी लिपि में लिखी जाती है और इसमें संयुक्ताक्षर होते हैं।",
		"विद्यार्थी पुस्तकालय में पढ़ने के लिए प्रतिदिन जाते हैं।",
		"क्षत्रिय, ज्ञान, श्रद्धा और द्वार जैसे शब्दों में जटिल रूप बनते हैं।",
	}},
	{"arabic", `"Noto Sans Arabic"`, "rtl", []string{
		"اللغة العربية من أكثر اللغات انتشاراً في العالم.",
		"يكتب النص العربي من اليمين إلى اليسار وتتصل الحروف ببعضها.",
		"ذهب الطالب إلى المكتبة ليقرأ كتاباً عن التاريخ والجغرافيا.",
		"تُستخدم الحركات مثل الفَتحة والضَّمة والكَسرة لتوضيح النطق.",
	}},
	{"cjk", `"Noto Sans JP"`, "", []string{
		"日本語の文章は漢字と仮名を組み合わせて書かれます。",
		"東京は日本の首都であり、多くの人々が暮らしています。",
		"組版の規則では、句読点の位置や行頭の禁則が定められています。",
		"カタカナは外来語の表記に使われることが多いです。",
	}},
	// Fallback-heavy: a Latin family with a word of another script in almost
	// every sentence, so each box's text misses in the primary face and is
	// walked cluster by cluster.
	{"mixed", `"Noto Sans", serif`, "", []string{
		"The word for peace is שלום in Hebrew and سلام in Arabic.",
		"In Hindi one says नमस्ते and in Japanese こんにちは when greeting.",
		"Greek αβγ, Armenian Աբգ and Georgian აბგ letters appear in this line.",
		"A Tibetan བོད་ word sits beside the Coptic ⲁⲃⲅ and the Ogham ᚁᚂᚃ.",
		"Prices: ₹100, ¥200 and ﷼300, with arrows → ⇒ and a ♞ chess knight.",
	}},
}

// rangedSheet declares two webfonts under one family, split by unicode-range,
// so that the family list is walked per grapheme cluster (namedFaceFor).
const rangedSheet = `@font-face { font-family: Split; src: url(NotoSans-Regular.ttf); unicode-range: U+0-24F }
@font-face { font-family: Split; src: url(NotoSansHebrew-Regular.ttf); unicode-range: U+590-5FF }
`

// variableRangedSheet is rangedSheet with the Latin face variable and the text
// asked for at a weight it is not drawn at by default, so that each face the
// family list gives is also an instance to look up.
const variableRangedSheet = `@font-face { font-family: Split; src: url(NotoSans-Variable.ttf); font-weight: 100 900; unicode-range: U+0-24F }
@font-face { font-family: Split; src: url(NotoSansHebrew-Regular.ttf); unicode-range: U+590-5FF }
p { font-weight: 450 }
`

// BenchmarkLayoutDocuments composes each document end to end.
func BenchmarkLayoutDocuments(b *testing.B) {
	set := benchFontSet(b)
	dir, _ := fonttest.NotoRoot()
	resolver, err := NewDirResolver(dir)
	if err != nil {
		b.Fatal(err)
	}
	for _, d := range benchDocuments {
		html := benchDocument(d.family, d.dir, d.sentences, 40)
		b.Run(d.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out := Compose(Input{HTML: html, Fonts: set}, Options{Page: A4})
				if len(out.Ops) == 0 {
					b.Fatal("nothing was drawn")
				}
			}
		})
	}
	// The mixed document in a family whose faces are restricted by
	// unicode-range, which is the path that asks the family list again for
	// every cluster.
	html := strings.Replace(benchDocument(`Split, serif`, "", benchDocuments[4].sentences, 40),
		"<style>", "<style>"+rangedSheet, 1)
	b.Run("ranged", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := Compose(Input{HTML: html, Fonts: set, Resources: resolver}, Options{Page: A4})
			if len(out.Ops) == 0 {
				b.Fatal("nothing was drawn")
			}
		}
	})
	// The same with a variable webfont set away from its default, so that
	// every cluster the family list answers is also asked for its instance.
	tmp := b.TempDir()
	for src, dst := range map[string]string{
		"../fonts/notosans/NotoSans-Variable.ttf":        "NotoSans-Variable.ttf",
		filepath.Join(dir, "NotoSansHebrew-Regular.ttf"): "NotoSansHebrew-Regular.ttf",
	} {
		data, err := os.ReadFile(src)
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, dst), data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	varResolver, err := NewDirResolver(tmp)
	if err != nil {
		b.Fatal(err)
	}
	varHTML := strings.Replace(benchDocument(`Split, serif`, "", benchDocuments[4].sentences, 40),
		"<style>", "<style>"+variableRangedSheet, 1)
	b.Run("ranged-variable", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out := Compose(Input{HTML: varHTML, Fonts: set, Resources: varResolver}, Options{Page: A4})
			if len(out.Ops) == 0 {
				b.Fatal("nothing was drawn")
			}
		}
	})
}
