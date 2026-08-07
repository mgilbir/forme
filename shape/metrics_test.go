package shape

import "testing"

// The metrics a font states rather than implies.
//
// Each of these is a number some other layer needs and had been guessing at: a
// line height without the leading term, an ex unit assumed to be half an em, an
// underline placed where it looked right across the standard fourteen. The
// values below are read out of the bundled face and checked against what
// fontTools reports for the same file, so a misread offset shows up here rather
// than as a rule drawn in the wrong place.

func TestDescriptorReadsTheMetricsTheFontStates(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	d := f.Descriptor()
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"LineGap", d.LineGap, 0},
		{"TypoAscent", d.TypoAscent, 1069},
		{"TypoDescent", d.TypoDescent, -293},
		{"TypoLineGap", d.TypoLineGap, 0},
		{"XHeight", d.XHeight, 536},
		{"UnderlinePosition", d.UnderlinePosition, -100},
		{"UnderlineThickness", d.UnderlineThickness, 50},
		{"StrikeoutPosition", d.StrikeoutPosition, 322},
		{"StrikeoutSize", d.StrikeoutSize, 50},
		{"Weight", d.Weight, 400},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if !d.UseTypoMetrics {
		t.Error("UseTypoMetrics is false; this face sets OS/2 fsSelection bit 7")
	}
}

// TestZeroAndUnknownAreDifferentAnswers is the point of Declared.
//
// The bundled face states a line gap of nothing, and the fourteen standard faces
// state nothing at all — they have no hhea, OS/2 or post table to state it in.
// Both report LineGap as 0, and a consumer that cannot tell them apart will read
// the second as an instruction. Declared is the only thing that distinguishes
// them, so it is checked in both directions.
func TestZeroAndUnknownAreDifferentAnswers(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	d := noto.Descriptor()
	if d.LineGap != 0 {
		t.Fatalf("this test assumes the bundled face declares a zero line gap, got %d", d.LineGap)
	}
	for _, m := range []struct {
		name string
		bit  Metric
	}{
		{"MetricLineGap", MetricLineGap}, {"MetricTypoMetrics", MetricTypoMetrics},
		{"MetricXHeight", MetricXHeight}, {"MetricCapHeight", MetricCapHeight},
		{"MetricUnderline", MetricUnderline}, {"MetricStrikeout", MetricStrikeout},
		{"MetricWeight", MetricWeight},
	} {
		if !d.Has(m.bit) {
			t.Errorf("the bundled face does not report %s, which it states", m.name)
		}
	}

	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatalf("loading Helvetica: %v", err)
	}
	s := std.Descriptor()
	if s.Declared != 0 {
		t.Errorf("a standard face declares %b; it has no table to declare any of it in", s.Declared)
	}
	if s.LineGap != 0 {
		t.Errorf("LineGap = %d for a standard face, want 0 with the bit clear", s.LineGap)
	}
	// The two answers a consumer has to tell apart, spelled out.
	if d.LineGap == s.LineGap && d.Has(MetricLineGap) == s.Has(MetricLineGap) {
		t.Error("a stated zero and an absent value are indistinguishable, which is the bug this guards")
	}
}

// TestAxesSayWhereTheOutlinesActuallySit covers what a variable face reports.
//
// The bundled face is variable, and what this module hands back is the outlines
// as stored — the default instance. Axes is how a caller finds that out; the
// default coordinate is where it landed.
func TestAxesSayWhereTheOutlinesActuallySit(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !f.IsVariable() {
		t.Fatal("the bundled face has fvar and is not reported as variable")
	}
	axes := f.Axes()
	want := map[string]Axis{
		"wght": {Tag: "wght", Min: 100, Default: 400, Max: 900},
		"wdth": {Tag: "wdth", Min: 62.5, Default: 100, Max: 100},
	}
	if len(axes) != len(want) {
		t.Fatalf("%d axes, want %d: %+v", len(axes), len(want), axes)
	}
	for _, a := range axes {
		w, ok := want[a.Tag]
		if !ok {
			t.Errorf("unexpected axis %q", a.Tag)
			continue
		}
		if a != w {
			t.Errorf("axis %s = %+v, want %+v", a.Tag, a, w)
		}
	}
	// wdth's range is the one that catches a 16.16 misread: 62.5 is not an
	// integer, so a reader that took the high word alone would say 62.
	for _, a := range axes {
		if a.Tag == "wdth" && a.Min != 62.5 {
			t.Errorf("wdth minimum = %v, want 62.5", a.Min)
		}
	}

	// Axes hands out a copy: a caller that scribbles on it must not reach the
	// face, which every document sharing this parse would then see.
	axes[0].Tag = "XXXX"
	if f.Axes()[0].Tag == "XXXX" {
		t.Error("Axes returned the face's own slice")
	}

	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatalf("loading Helvetica: %v", err)
	}
	if std.IsVariable() || std.Axes() != nil {
		t.Error("a standard face reports variation axes")
	}
}
