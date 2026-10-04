package shape

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Glyph.Cluster held to HarfBuzz's clusters over strings drawn from each
// script's characters, in the faces in the tree and in the corpora's where they
// are fetched: the scripts whose shapers reorder and insert by their own rules,
// emoji sequences and regional indicators, and runs a character nothing is
// drawn for cuts. The answers are checked in as clusters.expected.txt; see
// clusters.py, and cluster.go for what the clusters are.

// clusterFaces are where each face of clusters.expected.txt is: a path under
// the repository, or a corpus fetched to the directory a variable names.
var clusterFaces = map[string]struct{ env, path string }{
	"NotoSans-Variable.ttf":      {"", "../fonts/notosans/NotoSans-Variable.ttf"},
	"NotoSansArabic.ttf":         {"", filepath.Join(harfbuzzDir, "fonts", "NotoSansArabic.ttf")},
	"NotoSansKhmer.ttf":          {"", filepath.Join(harfbuzzDir, "fonts", "NotoSansKhmer.ttf")},
	"NotoSansJavanese.ttf":       {"", filepath.Join(harfbuzzDir, "fonts", "NotoSansJavanese.ttf")},
	"NotoSansBalinese.ttf":       {"", filepath.Join(harfbuzzDir, "fonts", "NotoSansBalinese.ttf")},
	"NotoSerifTibetan.ttf":       {"", filepath.Join(harfbuzzDir, "fonts", "NotoSerifTibetan.ttf")},
	"NotoSansHebrew-Regular.ttf": {"NOTO_FONTS", "NotoSansHebrew-Regular.ttf"},
	"Unifont-Regular.otf":        {"NOTO_FONTS", "Unifont-Regular.otf"},
	"NotoSansKR-Regular.otf":     {"NOTO_CJK", "NotoSansKR-Regular.otf"},
	"Noto-COLRv1.ttf":            {"EMOJI_FONTS", "Noto-COLRv1.ttf"},
}

func TestClustersAgreeWithHarfBuzz(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "clusters.expected.txt")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v; run `make hbclusters`", err)
	}
	defer file.Close()
	refuseUnpinnedOracle(t, path)
	var f *Face
	var name string
	compared, faces := 0, 0
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] == "face" {
			name, f = fields[1], nil
			where, ok := clusterFaces[name]
			if !ok {
				t.Fatalf("the expectations name %s, which this test does not know where to find", name)
			}
			file := where.path
			if where.env != "" {
				dir := os.Getenv(where.env)
				if dir == "" {
					t.Logf("%s: %s is not set, so it is not compared", name, where.env)
					continue
				}
				file = filepath.Join(dir, where.path)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != fields[2] {
				t.Fatalf("%s is %s, and the expectations were made from %s; run `make hbclusters`", name, got, fields[2])
			}
			if f, err = Load(data); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			faces++
			continue
		}
		if f == nil {
			continue
		}
		var text string
		for _, cp := range strings.Split(fields[0], ",") {
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			text += string(rune(r))
		}
		var want [][2]int
		for _, g := range fields[1:] {
			gid, cluster, ok := strings.Cut(g, "@")
			a, err1 := strconv.Atoi(gid)
			b, err2 := strconv.Atoi(cluster)
			if !ok || err1 != nil || err2 != nil {
				t.Fatalf("%q: %q is a glyph and a cluster", line, g)
			}
			want = append(want, [2]int{a, b})
		}
		glyphs, _ := f.ShapeGlyphs(text)
		var got [][2]int
		for _, g := range glyphs {
			got = append(got, [2]int{g.GID, g.Cluster})
		}
		compared++
		if !sameIntPairs(got, want) {
			t.Errorf("%s %s:\n  forme    %v\n  HarfBuzz %v", name, describeRunes(text), got, want)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if faces < 6 || compared < 2000 {
		t.Fatalf("%d faces and %d strings compared; the faces in the tree alone are six", faces, compared)
	}
}

func sameIntPairs(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
