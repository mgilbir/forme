package shape

import (
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// notoTTFs is every TrueType face in the fetched library, for the tests that
// make a claim about real fonts in general rather than about one of them.
//
// It fails rather than returning nothing when the library is there and holds no
// TrueType face: a sweep over an empty list passes without checking anything,
// which is the same silence fonttest.NotoDir exists to refuse.
func notoTTFs(t *testing.T) []string {
	t.Helper()
	dir := fonttest.NotoDir(t)
	names, err := filepath.Glob(filepath.Join(dir, "*.ttf"))
	if err != nil {
		t.Fatalf("reading the font library: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("%s holds the font library and no .ttf in it; `make noto-fonts` "+
			"fetches thirteen, and a sweep over none of them would pass having "+
			"read nothing", dir)
	}
	return names
}
