package layout

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// The two contracts a caller of this package has to know and could not read
// anywhere: what happens to a resolver's error, and who may touch a Recorder.

// sayingResolver returns the error it was built with, for every reference.
type sayingResolver struct {
	mu    sync.Mutex
	asked []string
	err   error
}

func (r *sayingResolver) Resolve(ref string) ([]byte, error) {
	r.mu.Lock()
	r.asked = append(r.asked, ref)
	r.mu.Unlock()
	return nil, r.err
}

// TestAResolversErrorIsShownToTheAuthor.
//
// The text of the error goes into a finding verbatim, and findings are what a
// document's author is shown. That is worth having a test for rather than a
// comment alone: it is the reason a resolver must not put a path, a host name or
// a credential into one, and a change that started summarising the error instead
// would silently make the documented warning false.
func TestAResolversErrorIsShownToTheAuthor(t *testing.T) {
	const secret = "open /srv/private/store/id_rsa: permission denied"
	res := &sayingResolver{err: errors.New(secret)}
	built := Build(Input{
		HTML:      `<p><img src="logo.png" alt="a logo"></p>`,
		Resources: res,
	})
	rec := NewRecorder(nil)
	Layout(built.Root, Size{W: picPx(600), H: picPx(800)}, nil, rec)

	found := false
	for _, f := range append(append([]Finding{}, built.Findings...), rec.Findings()...) {
		if strings.Contains(f.Message, secret) {
			found = true
		}
	}
	if !found {
		t.Errorf("the resolver's error text is not in any finding; resource.go "+
			"tells a caller that it is, and that is why it tells them what not "+
			"to put in one. Findings: %v", rec.Findings())
	}
	if len(res.asked) == 0 {
		t.Error("the resolver was never asked, so this test is watching nothing")
	}
}

// TestTwoDocumentsLayOutSideBySide is the other contract, and it is the race
// detector that reads it: two renders at once, each with its own Recorder, over
// one resolver the caller made safe. Nothing in this package starts a goroutine,
// so what this asserts is that nothing is shared behind the caller's back —
// under `make race` it is an assertion and not a wish.
func TestTwoDocumentsLayOutSideBySide(t *testing.T) {
	res := &sayingResolver{err: errors.New("not here")}
	const doc = `<p>the quick brown fox <img src="a.png" alt="a"> jumps over
		<span style="font-weight: bold">the lazy dog</span></p>
		<table><tr><td>one</td><td>two</td></tr></table>`

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			built := Build(Input{HTML: doc, Resources: res})
			rec := NewRecorder(nil)
			if frag := Layout(built.Root, Size{W: picPx(400), H: picPx(500)}, nil, rec); frag == nil {
				t.Error("a document laid out to nothing")
			}
		}()
	}
	wg.Wait()
}
