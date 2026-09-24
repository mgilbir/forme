package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Every go test this repository runs names its -timeout.
//
// go test's default is ten minutes per package binary, and nobody chose it for
// this repository. Layout's tests under the race detector, with every corpus in
// the environment, came to take about five hundred seconds of the six hundred,
// so the race job was one slow runner from failing on a limit nobody had
// written down. The Makefile names TEST_TIMEOUT and RACE_TIMEOUT, with the
// measurements they were chosen from, and the workflows name theirs.
//
// This holds every go test in the Makefile's recipes and in the workflows'
// run steps to naming one, so that the next target is not the one that
// forgets.

// goTestCommands is every logical line of text that runs go test, with its
// continuation lines joined, from a Makefile recipe or a workflow.
func goTestCommands(text string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		cur.WriteString(strings.TrimSuffix(trimmed, "\\"))
		cur.WriteByte(' ')
		if strings.HasSuffix(trimmed, "\\") {
			continue
		}
		if cmd := cur.String(); strings.Contains(cmd, "go test") {
			out = append(out, strings.TrimSpace(cmd))
		}
		cur.Reset()
	}
	return out
}

var timeoutFlag = regexp.MustCompile(`(^|\s)-(test\.)?timeout[ =]\S`)

func TestEveryGoTestNamesItsTimeout(t *testing.T) {
	files := []string{filepath.Join(root, "Makefile")}
	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, workflows...)
	seen := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, cmd := range goTestCommands(string(data)) {
			seen++
			if !timeoutFlag.MatchString(cmd) {
				t.Errorf("%s runs %q with go test's default timeout; name one "+
					"(TEST_TIMEOUT or RACE_TIMEOUT in the Makefile)", filepath.Base(f), cmd)
			}
		}
	}
	// The Makefile alone runs go test from a dozen targets; a scan that found
	// a handful has stopped reading them.
	if seen < 12 {
		t.Fatalf("only %d go test commands were found", seen)
	}
}

// TestTheTimeoutsAreDurations asks make what the two expand to, which is what
// every recipe gets, and requires durations go test reads and the headroom the
// Makefile's note gives them: the race run is five times slower than the
// ordinary one, and its bound is not smaller.
func TestTheTimeoutsAreDurations(t *testing.T) {
	needMake(t)
	test, err := time.ParseDuration(makeValue(t, "TEST_TIMEOUT"))
	if err != nil || test <= 10*time.Minute {
		t.Errorf("TEST_TIMEOUT is %v (%v); it must be a duration past go test's own "+
			"ten-minute default, or it names nothing the default did not", test, err)
	}
	race, err := time.ParseDuration(makeValue(t, "RACE_TIMEOUT"))
	if err != nil || race < test {
		t.Errorf("RACE_TIMEOUT is %v (%v), and TEST_TIMEOUT %v", race, err, test)
	}
}

// TestTheTimeoutCheckSeesACommand holds the scan to text it must refuse: a
// recipe split over continuation lines, a workflow step, and a comment that
// mentions go test and runs nothing.
func TestTheTimeoutCheckSeesACommand(t *testing.T) {
	text := "t:\n\tFOO=1 \\\n\t  go test -v -count=1 \\\n\t  ./x\n" +
		"# go test in a comment\n" +
		"u:\n\tgo test -timeout $(TEST_TIMEOUT) ./y\n" +
		"      - run: go test -run X -timeout=5m ./z\n"
	got := goTestCommands(text)
	if len(got) != 3 {
		t.Fatalf("found %q, want three commands", got)
	}
	var missing []string
	for _, c := range got {
		if !timeoutFlag.MatchString(c) {
			missing = append(missing, c)
		}
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "./x") {
		t.Errorf("the commands without a timeout are %q, want the one running ./x", missing)
	}
}
