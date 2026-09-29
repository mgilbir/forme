package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkShapeColdLatinLabel shapes a string no earlier iteration shaped: a
// chart label, thirty characters of Latin letters and digits, the case where
// every call is a miss of whatever a shaper might remember. It needs a Noto
// face for its GSUB and GPOS, and skips without the corpus.
func BenchmarkShapeColdLatinLabel(b *testing.B) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		b.Skip("NOTO_FONTS is not set")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSans-Regular.ttf"))
	if err != nil {
		b.Skip(err)
	}
	f, err := Load(data)
	if err != nil {
		b.Fatal(err)
	}
	labels := make([]string, b.N)
	for i := range labels {
		labels[i] = fmt.Sprintf("Quarterly revenue, region %06d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.ShapeGlyphs(labels[i])
	}
}
