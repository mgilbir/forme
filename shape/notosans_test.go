package shape

import (
	"os"
	"sync"
	"testing"
)

// The bundled face, for tests, read from the file the notosans package embeds.
//
// It is read rather than embedded because this package embeds no font: two
// megabytes in the package every caller imports, for a face most of them will
// not use, is a cost paid by everyone to serve a few. The fonts/notosans
// package embeds it for the callers that want it, and cannot be imported from
// here — it imports this one, and the tests are internal.
//
// The same face the other five come from, and the same way: a path.
var (
	notoOnceTest sync.Once
	notoTest     *Face
	notoTestErr  error
)

func NotoSans() (*Face, error) {
	notoOnceTest.Do(func() {
		data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
		if err != nil {
			notoTestErr = err
			return
		}
		notoTest, notoTestErr = Load(data)
	})
	if notoTestErr != nil {
		return nil, notoTestErr
	}
	return notoTest.Clone(), nil
}

// notoSansBytes is the face's own bytes, for a test that reads the file rather
// than the face.
func notoSansBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatalf("reading the bundled face: %v", err)
	}
	return data
}

// NotoSansSimple is the same face read as a simple font, which some tests need
// because a simple font addresses glyphs by character code rather than by index.
func NotoSansSimple() (*Face, error) {
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		return nil, err
	}
	return LoadSimple(data)
}
