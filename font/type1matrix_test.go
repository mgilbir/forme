package font

import "testing"

// TestAType1FontMatrixIsReadAsPostScript. The matrix is cleartext PostScript,
// and its numbers are PostScript's: "1e-3" is how a font writes a thousandth
// as readily as "0.001". It was read with the CFF BCD reader, which knows only
// an upper-case E and a leading minus, so "1e-3" came out as 13 — every width
// scaled by thirteen thousand.
func TestAType1FontMatrixIsReadAsPostScript(t *testing.T) {
	for _, tc := range []struct {
		matrix string
		want   float64
	}{
		{"[0.001 0 0 0.001 0 0]", 0.001},
		{"[1e-3 0 0 1e-3 0 0]", 0.001},
		{"[1E-3 0 0 1E-3 0 0]", 0.001},
		{"[.0005 0 0 .0005 0 0]", 0.0005},
		// A radix number is PostScript and not a number strconv reads, and
		// no scale is the answer that leaves the caller's default standing.
		{"[8#17 0 0 1 0 0]", 0},
		{"[/x 0 0 1 0 0]", 0},
	} {
		got := extractType1FontMatrix([]byte("%!PS-AdobeFont-1.0\n/FontMatrix " + tc.matrix + " readonly def\n"))
		if got != tc.want {
			t.Errorf("%s read as %v, want %v", tc.matrix, got, tc.want)
		}
	}
}
