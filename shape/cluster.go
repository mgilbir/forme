package shape

// Clusters, as HarfBuzz forms and merges them at its default cluster level,
// HB_BUFFER_CLUSTER_LEVEL_MONOTONE_GRAPHEMES.
//
// A glyph's Cluster starts as the byte offset of the character it is for. Two
// things then make clusters coarser than characters, and both are what a
// caller mapping the page back to the text needs:
//
//   - The characters of one grapheme — a letter and its marks, an emoji and
//     its skin tone, a family joined by zero width joiners, a pair of regional
//     indicators — are one cluster from the start (hb_form_clusters), so that
//     no caret can land between a letter and its accent.
//   - A substitution that takes several glyphs into one, a reordering that
//     moves glyphs past others, and a glyph taken out, merge the clusters
//     they touch (merge_clusters), so that the clusters stay in the order of
//     the text and a glyph is never in a cluster that starts after a later
//     glyph's.
//
// A merge reaches past the glyphs it is asked about to the glyphs either side
// that share a cluster with the one at that edge: a decomposition's parts are
// all of one cluster, and a ligature of the first of them takes the rest. A
// syllable shaped on its own still reaches into the syllables beside it,
// through clusterEdges, as HarfBuzz, which shapes the run as one buffer, does.
//
// Which characters continue a grapheme is graphemeContinues (trak.go).

// formClusters is hb_form_clusters: each grapheme's glyphs, from one that
// does not continue the one before it to the next that does not, given the
// grapheme's first cluster.
//
// HarfBuzz forms them before anything is put into the text or moved, and
// these glyphs are formed after; joins says, for what was, whether a glyph
// continues the one before it as HarfBuzz's order would have it. A dotted
// circle the shaper put before a vowel sign (markInvalidVowels) is passed
// over, so the sign stays in its vowel's grapheme, and takes the sign's
// cluster; a Hangul tone mark moved in front of its syllable (hangul.go) is
// already of the syllable's cluster, and continues nothing before it.
func formClusters(buf []Glyph, joins func(i int) (join, decided bool)) {
	for start := 0; start < len(buf); {
		end := start + 1
		for end < len(buf) {
			join, decided := joins(end)
			if !decided {
				join = buf[end].cont
			}
			if !join {
				break
			}
			end++
		}
		mergeClusters(nil, buf, start, end)
		start = end
	}
}

// clusterEdges is what lies either side of a syllable shaped on its own: the
// glyphs of the run before it, already shaped, and those after it, not yet. A
// merge that reaches the syllable's edge goes on into them.
type clusterEdges struct {
	before, after []Glyph
}

// mergeClusters is merge_clusters with no output beside the glyphs, as a
// reordering merges: the glyphs from start to end made one cluster, the
// smallest of theirs, with those after the last and before the first that
// shared a cluster with it.
func mergeClusters(e *clusterEdges, buf []Glyph, start, end int) {
	mergeClustersAt(e, nil, buf, 0, start, end, true)
}

// mergeClustersAt is merge_clusters_impl. in is what is still to be looked at
// and idx the position in it; settled is what a pass has finished with, which
// with in[:idx] is HarfBuzz's output. inPlace is a merge with no output,
// whose position is the run's start, so that the first glyph's cluster is
// followed back as far as it goes.
func mergeClustersAt(e *clusterEdges, settled, in []Glyph, idx, start, end int, inPlace bool) {
	if end-start < 2 {
		return
	}
	cluster := in[start].Cluster
	for i := start + 1; i < end; i++ {
		cluster = min(cluster, in[i].Cluster)
	}
	var before, after []Glyph
	if e != nil {
		before, after = e.before, e.after
	}
	// Past the last glyph, while the cluster goes on.
	if cluster != in[end-1].Cluster {
		last := in[end-1].Cluster
		for end < len(in) && in[end].Cluster == last {
			end++
		}
		if end == len(in) {
			for i := 0; i < len(after) && after[i].Cluster == last; i++ {
				after[i].Cluster = cluster
			}
		}
	}
	// Before the first, back to the position, and with no output past it.
	first := in[start].Cluster
	if cluster != first {
		for idx < start && in[start-1].Cluster == first {
			start--
		}
		if inPlace && start == 0 {
			for i := len(before); i > 0 && before[i-1].Cluster == first; i-- {
				before[i-1].Cluster = cluster
			}
		}
	}
	// And at the position, into the output.
	if !inPlace && idx == start && first != cluster {
		out := outView{before, settled, in[:idx]}
		for i := out.len(); i > 0 && out.at(i-1).Cluster == first; i-- {
			out.at(i - 1).Cluster = cluster
		}
	}
	for i := start; i < end; i++ {
		in[i].Cluster = cluster
	}
}

// outView is HarfBuzz's output buffer as a pass here holds it: the syllables
// before this one, what the pass has settled, then the glyphs of what is
// pending before the position it is at, which HarfBuzz would have moved into
// its output by then.
type outView struct {
	before, settled, pending []Glyph
}

func (o outView) len() int { return len(o.before) + len(o.settled) + len(o.pending) }

func (o outView) at(i int) *Glyph {
	if i < len(o.before) {
		return &o.before[i]
	}
	i -= len(o.before)
	if i < len(o.settled) {
		return &o.settled[i]
	}
	return &o.pending[i-len(o.settled)]
}

// deleteClusterInPlace is the cluster half of delete_glyphs_inplace, for
// the glyph at i of a run being compacted, kept is how many of the glyphs
// before it are kept, and buf[:kept] are those: the glyph's cluster survives
// where the glyph after it shares it; else it is given to the cluster of the
// last glyph kept, where its own is the earlier; and with nothing kept before
// it, merged into the glyph after it.
func deleteClusterInPlace(buf []Glyph, i, kept int) {
	cluster := buf[i].Cluster
	if i+1 < len(buf) && cluster == buf[i+1].Cluster {
		return
	}
	if kept > 0 {
		if old := buf[kept-1].Cluster; cluster < old {
			for k := kept; k > 0 && buf[k-1].Cluster == old; k-- {
				buf[k-1].Cluster = cluster
			}
		}
		return
	}
	if i+1 < len(buf) {
		mergeClusters(nil, buf[i:], 0, 2)
	}
}

// deleteClusterAt is what delete_glyph does to clusters before the glyph of
// in at idx is taken out: nothing where another glyph still stands for its
// cluster beside it; else its cluster given to the glyphs of the cluster
// before it, where its own is the earlier; and with nothing before it, merged
// into the glyph after it.
func deleteClusterAt(e *clusterEdges, settled, in []Glyph, idx int) {
	cluster := in[idx].Cluster
	var before, after []Glyph
	if e != nil {
		before, after = e.before, e.after
	}
	out := outView{before, settled, in[:idx]}
	n := out.len()
	next := -1
	switch {
	case idx+1 < len(in):
		next = in[idx+1].Cluster
	case len(after) > 0:
		next = after[0].Cluster
	}
	if next == cluster || n > 0 && cluster == out.at(n-1).Cluster {
		return
	}
	if n > 0 {
		if old := out.at(n - 1).Cluster; cluster < old {
			for i := n; i > 0 && out.at(i-1).Cluster == old; i-- {
				out.at(i - 1).Cluster = cluster
			}
		}
		return
	}
	if idx+1 < len(in) {
		mergeClustersAt(e, settled, in, idx, idx, idx+2, false)
	} else if len(after) > 0 {
		// The glyph after it is the next syllable's.
		old := after[0].Cluster
		for i := 0; i < len(after) && after[i].Cluster == old; i++ {
			after[i].Cluster = min(old, cluster)
		}
	}
}
