package shape

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

// Face.ID: what a backend keys a face by, since layout hands every document
// faces of its own. See FaceID.

func readNotoSans(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// loaded is the face a constructor returned, failing t where it returned an
// error: loaded(t)(Load(data)).
func loaded(t *testing.T) func(*Face, error) *Face {
	return func(f *Face, err error) *Face {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
}

func TestACloneHasItsFacesID(t *testing.T) {
	face := loaded(t)(Load(readNotoSans(t)))
	// Cloned before the ID is first asked for, and after, since the hash is
	// made the first time and shared.
	before := face.Clone()
	id := face.ID()
	after := face.Clone()
	if before.ID() != id || after.ID() != id {
		t.Errorf("a clone's ID differs from its face's: %v and %v, want %v", before.ID(), after.ID(), id)
	}
	before.Encode("Hi")
	if before.ID() != id {
		t.Error("a clone's ID changed when it was used")
	}
}

func TestTheSameFileLoadedAgainHasTheSameID(t *testing.T) {
	data := readNotoSans(t)
	a := loaded(t)(Load(data))
	b := loaded(t)(Load(bytes.Clone(data)))
	if a.ID() != b.ID() {
		t.Errorf("one file loaded twice has two IDs: %v and %v", a.ID(), b.ID())
	}
	other := loaded(t)(Load(readFont(t, "../testdata/harfbuzz/aat/fonts/TestAATMorx.ttf")))
	if other.ID() == a.ID() {
		t.Error("two different fonts have one ID")
	}
}

func readFont(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAnInstanceIsIdentifiedByWhereItWasCut(t *testing.T) {
	data := readNotoSans(t)
	def := loaded(t)(Load(data))
	bold := loaded(t)(LoadInstance(data, map[string]float64{"wght": 700}))
	boldAgain := loaded(t)(LoadInstance(bytes.Clone(data), map[string]float64{"wght": 700}))
	light := loaded(t)(LoadInstance(data, map[string]float64{"wght": 300}))
	if bold.ID() != boldAgain.ID() {
		t.Errorf("one instance cut twice has two IDs: %v and %v", bold.ID(), boldAgain.ID())
	}
	if bold.ID() == light.ID() || bold.ID() == def.ID() || light.ID() == def.ID() {
		t.Errorf("instances at different locations share an ID: default %v, 700 %v, 300 %v",
			def.ID(), bold.ID(), light.ID())
	}
}

func TestFeatureSettingsAreInTheID(t *testing.T) {
	face := loaded(t)(Load(readNotoSans(t)))
	smcp := face.WithFeatureSettings([]FeatureSetting{{Tag: "smcp", On: true}})
	smcpAgain := face.WithFeatureSettings([]FeatureSetting{{Tag: "smcp", On: true}})
	noKern := face.WithFeatureSettings([]FeatureSetting{{Tag: "kern", On: false}})
	if smcp.ID() == face.ID() || noKern.ID() == face.ID() || smcp.ID() == noKern.ID() {
		t.Error("faces that shape differently share an ID")
	}
	if smcp.ID() != smcpAgain.ID() {
		t.Error("the same settings asked for twice give two IDs")
	}
}

func TestASimpleFaceIsNotTheCompositeOne(t *testing.T) {
	data := readNotoSans(t)
	simple := loaded(t)(LoadSimple(data))
	composite := loaded(t)(Load(data))
	if simple.ID() == composite.ID() {
		t.Error("a face embedded as a simple font has the ID of the composite one")
	}
}

func TestStandardFacesAreIdentifiedByName(t *testing.T) {
	a := loaded(t)(Standard("Helvetica"))
	b := loaded(t)(Standard("Helvetica"))
	c := loaded(t)(Standard("Times-Roman"))
	if a.ID() != b.ID() {
		t.Error("one standard face loaded twice has two IDs")
	}
	if a.ID() == c.ID() {
		t.Error("two standard faces share an ID")
	}
}

func TestAFaceOfACollectionIsItsOwn(t *testing.T) {
	data := readFont(t, "../font/testdata/woff2-collections/distinct.ttc")
	zero := loaded(t)(LoadCollection(data, 0))
	zeroAgain := loaded(t)(LoadCollection(bytes.Clone(data), 0))
	one := loaded(t)(LoadCollection(data, 1))
	if zero.ID() != zeroAgain.ID() {
		t.Error("one face of a collection loaded twice has two IDs")
	}
	if zero.ID() == one.ID() {
		t.Error("two faces of a collection share an ID")
	}
}

// Clones share the hash, and are documented as used from separate goroutines
// (separate documents laid out at once), so the first ID may be asked for by
// several at the same time. Run under -race.
func TestClonesAskedForTheirIDAtOnceAgree(t *testing.T) {
	face := loaded(t)(Load(readNotoSans(t)))
	ids := make([]FaceID, 8)
	var wg sync.WaitGroup
	for i := range ids {
		c := face.Clone()
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i] = c.ID()
		}()
	}
	wg.Wait()
	for i := range ids {
		if ids[i] != face.ID() {
			t.Errorf("clone %d has %v, want %v", i, ids[i], face.ID())
		}
	}
}
