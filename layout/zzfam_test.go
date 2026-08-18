package layout

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestProbeFam(t *testing.T) {
	want := os.Getenv("FAMILY")
	if want == "" {
		t.Skip("set FAMILY")
	}
	root := wptDir(t)
	for _, rt := range findReftests(t, root) {
		if !strings.Contains(rt.test, want) {
			continue
		}
		got, _, _, err := renderForCompareDetail(root, rt.test)
		if err != nil {
			continue
		}
		ref, _, _, err := renderForCompareDetail(root, rt.ref)
		if err != nil {
			continue
		}
		if pictureEqual(got, ref, pageClip()) == rt.mismatch {
			fmt.Printf("FAIL %s\n", rt.test[strings.LastIndex(rt.test, "/")+1:])
		}
	}
}
