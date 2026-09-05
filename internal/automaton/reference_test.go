package automaton

import (
	"slices"
	"unicode/utf8"

	"github.com/kirklin/go-swd/internal/normalize"
)

// ref is a deliberately simple Aho-Corasick over folded runes with map-based
// nodes, used as the test oracle for exact-mode scans.
type refNode struct {
	kids  map[rune]*refNode
	fail  *refNode
	word  int
	depth int
}

type ref struct {
	root  *refNode
	words []Word
}

func foldWord(text string) []rune {
	var out []rune
	for _, r := range text {
		if normalize.Class(r)&normalize.Ignorable != 0 {
			continue
		}
		out = append(out, normalize.Fold(r))
	}
	return out
}

func buildRef(words []Word) *ref {
	rf := &ref{root: &refNode{kids: map[rune]*refNode{}, word: -1}}
	for _, w := range words {
		rs := foldWord(w.Text)
		if len(rs) == 0 {
			continue
		}
		cur := rf.root
		for d, r := range rs {
			nx, ok := cur.kids[r]
			if !ok {
				nx = &refNode{kids: map[rune]*refNode{}, word: -1, depth: d + 1}
				cur.kids[r] = nx
			}
			cur = nx
		}
		if cur.word < 0 {
			cur.word = len(rf.words)
			rf.words = append(rf.words, w)
		} else {
			rf.words[cur.word].Payload |= w.Payload
		}
	}
	queue := []*refNode{}
	for _, k := range rf.root.kids {
		k.fail = rf.root
		queue = append(queue, k)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for r, k := range cur.kids {
			queue = append(queue, k)
			f := cur.fail
			for f != nil {
				if nx, ok := f.kids[r]; ok {
					k.fail = nx
					break
				}
				f = f.fail
			}
			if f == nil {
				k.fail = rf.root
			}
		}
	}
	return rf
}

func (rf *ref) matchAll(text string, mask uint32) []Hit {
	// effective sequence: folded, ignorables dropped, with position maps
	var (
		seq     []rune
		runeIdx []int
		bStart  []int
		bEnd    []int
	)
	i := 0
	for off := 0; off < len(text); {
		r, w := utf8.DecodeRuneInString(text[off:])
		if normalize.Class(r)&normalize.Ignorable == 0 {
			seq = append(seq, normalize.Fold(r))
			runeIdx = append(runeIdx, i)
			bStart = append(bStart, off)
			bEnd = append(bEnd, off+w)
		}
		i++
		off += w
	}
	var hits []Hit
	cur := rf.root
	for k, r := range seq {
		for cur != rf.root && cur.kids[r] == nil {
			cur = cur.fail
		}
		if nx, ok := cur.kids[r]; ok {
			cur = nx
		} else {
			continue
		}
		for nd := cur; nd != rf.root; nd = nd.fail {
			if nd.word >= 0 && (mask == 0 || rf.words[nd.word].Payload&mask != 0) {
				s := k - nd.depth + 1
				hits = append(hits, Hit{Word: int32(nd.word), StartRune: runeIdx[s], EndRune: runeIdx[k] + 1, StartByte: bStart[s], EndByte: bEnd[k]})
			}
		}
	}
	return hits
}

func sortHits(h []Hit) []Hit {
	h = slices.Clone(h)
	slices.SortFunc(h, func(a, b Hit) int {
		if a.StartRune != b.StartRune {
			return a.StartRune - b.StartRune
		}
		if a.EndRune != b.EndRune {
			return a.EndRune - b.EndRune
		}
		return int(a.Word - b.Word)
	})
	return h
}
