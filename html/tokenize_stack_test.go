package html

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
)

// stackChildEnv names the environment variable that tells the helper below it
// is the child process rather than the parent.
const stackChildEnv = "FORME_HTML_STACK_CHILD"

// TestSkippedConstructsDoNotGrowTheStack is the guard on a defect that no other
// kind of test can hold.
//
// Every construct the tokenizer drops rather than tokenizes — a comment, a
// declaration, a processing instruction, a stray "<", a nameless end tag — used
// to ask for the next token by calling next again. One frame pair per dropped
// construct, and a document is allowed to be 64 MB of them: fourteen megabytes
// of comments produced "fatal error: stack overflow", which is not a panic. No
// recover sees it, the process dies, and on a server that is every other
// document in flight as well.
//
// So the assertion cannot be made in this process: the failure it looks for
// kills the test binary before it can report anything. It runs in a child with
// debug.SetMaxStack cut to a megabyte, which is what makes the input small
// enough to be cheap — a hundred thousand dropped constructs need tens of
// megabytes of stack under the old shape and one frame under this one. The
// parent asserts the child ran the helper and exited cleanly.
func TestSkippedConstructsDoNotGrowTheStack(t *testing.T) {
	if os.Getenv(stackChildEnv) != "" {
		t.Skip("this is the child process; the helper below does the work")
	}
	const helper = "TestSkippedConstructsDoNotGrowTheStackHelper"
	cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$", "-test.v")
	cmd.Env = append(os.Environ(), stackChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tokenizing a document of dropped constructs failed in a child process "+
			"with a one-megabyte stack limit: %v\n%s", err, firstLines(string(out), 20))
	}
	if !strings.Contains(string(out), "--- PASS: "+helper) {
		t.Fatalf("the child process did not run %s:\n%s", helper, firstLines(string(out), 20))
	}
}

// firstLines keeps a child's failure readable: a stack overflow dumps every
// goroutine in the process, and the first line of it is the one that says what
// happened.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

// TestSkippedConstructsDoNotGrowTheStackHelper is the child half of the test
// above. It does nothing when run directly, because the stack limit it needs is
// set by the parent's re-execution and not by go test.
func TestSkippedConstructsDoNotGrowTheStackHelper(t *testing.T) {
	if os.Getenv(stackChildEnv) == "" {
		t.Skip("run by TestSkippedConstructsDoNotGrowTheStack, not on its own")
	}
	debug.SetMaxStack(1 << 20)

	// Each of these produces no token and no text, so each one used to be a
	// frame pair that lived until the next token was found.
	for _, tc := range []struct{ name, construct string }{
		{"comments", "<!---->"},
		{"unclosed comments", "<!--"},
		{"declarations", "<!x>"},
		{"processing instructions", "<?x?>"},
		{"stray less-than signs", "<"},
		{"nameless end tags", "</ >"},
	} {
		src := strings.Repeat(tc.construct, 100000) + "<p>x</p>"
		doc, _, _ := Parse(src)
		if doc == nil {
			t.Fatalf("%s: %d bytes produced no tree", tc.name, len(src))
		}
	}
}

// TestEveryDroppedConstructIsConsumed is the other half of the loop's safety.
//
// next goes round again whenever a construct produced no token, so a branch
// that reports "nothing produced" without moving pos would spin for ever rather
// than overflow — a worse failure than the one being fixed, and one a scale
// test cannot see. Each such branch is checked here directly: it must leave pos
// further on than it found it.
func TestEveryDroppedConstructIsConsumed(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a closed comment", "<!---->"},
		{"a comment that is never closed", "<!--x"},
		{"a declaration", "<!x>"},
		{"a declaration with no terminator", "<!x"},
		{"a CDATA-shaped declaration", "<![CDATA[x]]>"},
		{"a processing instruction", "<?x?>"},
		{"a processing instruction with no terminator", "<?x"},
		{"a stray less-than sign", "<="},
		{"a stray less-than sign at the end", "<"},
		{"a nameless end tag", "</ >"},
		{"a nameless end tag with no terminator", "</ "},
	} {
		tk := newTokenizer(tc.src)
		before := tk.pos
		if _, ok := tk.step(); ok {
			t.Errorf("%s: produced a token; this test is about the branches that do not", tc.name)
			continue
		}
		if tk.pos <= before {
			t.Errorf("%s: produced no token and left pos at %d, so next would spin for ever",
				tc.name, tk.pos)
		}
	}
}
