package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// BenchmarkLoad is what a caller that loads a font to subset it once pays
// before the subset (#990).
func BenchmarkLoad(b *testing.B) {
	for _, tc := range []struct {
		name string
		data func(testing.TB) []byte
	}{
		{"NotoSans-Regular", func(t testing.TB) []byte { return fonttest.NotoFile(t, "NotoSans-Regular.ttf") }},
		{"NotoSansDevanagari-Regular", func(t testing.TB) []byte { return fonttest.NotoFile(t, "NotoSansDevanagari-Regular.ttf") }},
		{"ipag", func(t testing.TB) []byte { return fonttest.NotoFile(t, "ipag.ttf") }},
		{"SourceSans3-Regular", func(t testing.TB) []byte { return fonttest.CFFFile(t, "SourceSans3-Regular.otf") }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			data := tc.data(b)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Load(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
