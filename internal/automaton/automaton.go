// Package automaton implements a cache-friendly Aho-Corasick automaton over
// folded runes. It is the matching core of go-swd.
//
// Layout: nodes are numbered in BFS order and stored in one flat slice. The
// root and other high-fanout nodes use dense transition rows indexed by
// character code; the rest use sorted edge lists. Every node carries a fail
// link and an "out" link (the nearest proper suffix that is a word), so a scan
// only visits nodes that actually end a word.
//
// Characters are mapped to small codes through a 64K-entry table that already
// applies normalize.Fold, so case, width and digit-style folding cost nothing
// at scan time and positions always refer to the original text. Characters
// that appear in no word map to code 0 and reset the scan to the root.
//
// A 2-gram bitmap prefilter lets the scanner skip, in one bit test, positions
// where no word can start.
package automaton

import (
	"errors"
	"fmt"
	"math/bits"
	"slices"
	"unicode/utf8"

	"github.com/kirklin/go-swd/internal/normalize"
)

const (
	codeMask uint16 = 0x3FFF
	flagSep  uint16 = 0x4000 // separator: may be skipped in gap mode
	flagIgn  uint16 = 0x8000 // ignorable: always skipped

	maxCodes = int(codeMask)

	// MaxWordLen is the maximum word length in runes after folding.
	MaxWordLen = 255

	denseThreshold = 16
)

var (
	// ErrWordTooLong is returned by Build for a word longer than MaxWordLen runes.
	ErrWordTooLong = errors.New("automaton: word longer than 255 runes")
	// ErrAlphabetTooLarge is returned by Build when the words use more than
	// 16383 distinct (folded) characters.
	ErrAlphabetTooLarge = errors.New("automaton: more than 16383 distinct characters")
)

// Word is a dictionary entry. Payload is opaque to the automaton (go-swd
// stores the category bitmask there). When several words fold to the same
// character sequence they are merged: the first Text is kept and the
// payloads are OR-ed together.
type Word struct {
	Text    string
	Payload uint32
}

// Hit is one match. Rune positions are indices into []rune(text); byte
// positions are offsets into text. The span [StartByte, EndByte) covers the
// original characters of the match, including any skipped separators.
type Hit struct {
	Word      int32 // index into Matcher.Words()
	StartRune int
	EndRune   int
	StartByte int
	EndByte   int
}

// Options control one scan.
type Options struct {
	// Mask filters words by payload: only words with Payload&Mask != 0 are
	// reported. Mask == 0 reports every word.
	Mask uint32
	// MaxGap is the maximum number of consecutive separator characters
	// (whitespace, punctuation, symbols) tolerated between two characters of
	// a word, e.g. 1 matches "f*u*c*k". 0 means exact matching.
	MaxGap int
	// CollapseRepeats treats a repeated character as part of the previous
	// one when it cannot continue any word, so "fuuuck" matches "fuck" while
	// "妈妈" still matches "妈妈".
	CollapseRepeats bool
}

type node struct {
	edgeStart int32
	edgeEnd   int32
	dense     int32 // offset of the dense row in denseNext, or -1
	fail      int32
	out       int32 // nearest proper suffix node that is a word, or -1
	word      int32 // word index, or -1
	depth     uint16
}

// Matcher is an immutable compiled automaton. It is safe for concurrent use.
type Matcher struct {
	bmp       *[1 << 16]uint16 // per BMP rune: code | flags (folding applied)
	ext       map[rune]uint16  // astral runes that have a code
	nCodes    int32            // codes are 1..nCodes-1; 0 = not in any word
	nodes     []node
	edgeCode  []uint16
	edgeNext  []int32
	denseNext []int32 // rows of nCodes entries; row 0 belongs to the root
	pre       []uint64
	preShift  uint32
	words     []Word
	maxDepth  int
	dense     int
}

// Stats describes the size of a compiled Matcher.
type Stats struct {
	Words      int
	Nodes      int
	Edges      int
	DenseNodes int
	Alphabet   int
	MaxLen     int
	Bytes      int // approximate heap footprint of the tables (words excluded)
}

// Build compiles words into a Matcher. Empty words (after dropping ignorable
// characters) are skipped.
func Build(words []Word) (*Matcher, error) {
	fold, class := normalize.BMP()
	m := &Matcher{}

	// Pass 1: fold every word into a shared rune arena, collect the alphabet.
	type entry struct{ off, n, src int32 }
	arena := make([]rune, 0, 8*len(words))
	entries := make([]entry, 0, len(words))
	inAlpha := make([]bool, 1<<16)
	var astral []rune
	for i, w := range words {
		start := len(arena)
		for _, r := range w.Text {
			var f rune
			var cl uint8
			if r < 1<<16 {
				f, cl = rune(fold[r]), class[r]
			} else {
				f, cl = normalize.Fold(r), normalize.Class(r)
			}
			if cl&normalize.Ignorable != 0 {
				continue
			}
			arena = append(arena, f)
			if f < 1<<16 {
				inAlpha[f] = true
			} else {
				astral = append(astral, f)
			}
		}
		n := len(arena) - start
		if n == 0 {
			continue
		}
		if n > MaxWordLen {
			return nil, fmt.Errorf("%w: %q", ErrWordTooLong, w.Text)
		}
		entries = append(entries, entry{int32(start), int32(n), int32(i)})
	}

	// Codes 1..K in code point order of the folded rune.
	alpha := make([]rune, 0, 4096)
	for r := 0; r < 1<<16; r++ {
		if inAlpha[r] {
			alpha = append(alpha, rune(r))
		}
	}
	if len(astral) > 0 {
		slices.Sort(astral)
		alpha = append(alpha, slices.Compact(astral)...)
	}
	if len(alpha) > maxCodes {
		return nil, ErrAlphabetTooLarge
	}
	m.nCodes = int32(len(alpha) + 1)
	codeOf := make([]uint16, 1<<16) // folded BMP rune -> code
	codeOfExt := map[rune]uint16{}
	for i, r := range alpha {
		c := uint16(i + 1)
		if r < 1<<16 {
			codeOf[r] = c
		} else {
			codeOfExt[r] = c
		}
	}
	code := func(f rune) uint16 {
		if f < 1<<16 {
			return codeOf[f]
		}
		return codeOfExt[f]
	}

	// Pass 2: temporary trie (first child / next sibling), insertion order.
	type tnode struct {
		child, sibling int32
		word           int32
		code           uint16
		depth          uint16
	}
	tn := make([]tnode, 1, len(arena)+1)
	tn[0] = tnode{child: -1, sibling: -1, word: -1}
	rootKids := make([]int32, m.nCodes)
	for i := range rootKids {
		rootKids[i] = -1
	}
	m.words = make([]Word, 0, len(entries))
	for _, e := range entries {
		cur := int32(0)
		for k := int32(0); k < e.n; k++ {
			c := code(arena[e.off+k])
			nx := int32(-1)
			if cur == 0 {
				nx = rootKids[c]
			} else {
				for s := tn[cur].child; s >= 0; s = tn[s].sibling {
					if tn[s].code == c {
						nx = s
						break
					}
				}
			}
			if nx < 0 {
				nx = int32(len(tn))
				tn = append(tn, tnode{child: -1, sibling: -1, word: -1, code: c, depth: uint16(k + 1)})
				if cur == 0 {
					rootKids[c] = nx
				} else {
					tn[nx].sibling = tn[cur].child
					tn[cur].child = nx
				}
			}
			cur = nx
		}
		src := words[e.src]
		if tn[cur].word < 0 {
			tn[cur].word = int32(len(m.words))
			m.words = append(m.words, src)
		} else {
			m.words[tn[cur].word].Payload |= src.Payload
		}
	}

	// Pass 3: BFS renumbering into the flat layout, fail links on the fly.
	n := int32(len(tn))
	m.nodes = make([]node, n)
	m.edgeCode = make([]uint16, 0, n)
	m.edgeNext = make([]int32, 0, n)
	m.denseNext = make([]int32, m.nCodes) // root row
	for i := range m.denseNext {
		m.denseNext[i] = -1
	}
	m.nodes[0] = node{dense: 0, out: -1, word: -1}
	final := make([]int32, n)
	queue := make([]int32, 0, n)
	queue = append(queue, 0)
	nextID := int32(1)
	var kids []int32
	for qi := 0; qi < len(queue); qi++ {
		t := queue[qi]
		f := final[t]
		kids = kids[:0]
		if t == 0 {
			for c := int32(1); c < m.nCodes; c++ {
				if k := rootKids[c]; k >= 0 {
					kids = append(kids, k)
				}
			}
		} else {
			for s := tn[t].child; s >= 0; s = tn[s].sibling {
				kids = append(kids, s)
			}
			if len(kids) > 1 {
				slices.SortFunc(kids, func(a, b int32) int { return int(tn[a].code) - int(tn[b].code) })
			}
		}
		m.nodes[f].edgeStart = int32(len(m.edgeCode))
		for _, k := range kids {
			kf := nextID
			nextID++
			final[k] = kf
			c := tn[k].code
			m.nodes[kf] = node{dense: -1, out: -1, word: tn[k].word, depth: tn[k].depth}
			if t == 0 {
				m.denseNext[c] = kf
				continue
			}
			m.edgeCode = append(m.edgeCode, c)
			m.edgeNext = append(m.edgeNext, kf)
			x := m.nodes[f].fail
			for {
				if nx := m.next(x, c); nx >= 0 {
					m.nodes[kf].fail = nx
					break
				}
				if x == 0 {
					break
				}
				x = m.nodes[x].fail
			}
		}
		m.nodes[f].edgeEnd = int32(len(m.edgeCode))
		queue = append(queue, kids...)
	}

	// Out links: BFS order guarantees fail[i] < i.
	for i := int32(1); i < n; i++ {
		nd := &m.nodes[i]
		if fl := nd.fail; fl > 0 {
			if m.nodes[fl].word >= 0 {
				nd.out = fl
			} else {
				nd.out = m.nodes[fl].out
			}
		}
		if int(nd.depth) > m.maxDepth {
			m.maxDepth = int(nd.depth)
		}
	}

	// Dense rows for high-fanout nodes, within a memory budget.
	var cand []int32
	for i := int32(1); i < n; i++ {
		if m.nodes[i].edgeEnd-m.nodes[i].edgeStart >= denseThreshold {
			cand = append(cand, i)
		}
	}
	slices.SortFunc(cand, func(a, b int32) int {
		fa := m.nodes[a].edgeEnd - m.nodes[a].edgeStart
		fb := m.nodes[b].edgeEnd - m.nodes[b].edgeStart
		return int(fb - fa)
	})
	budget := int32(4*len(m.edgeCode)) + 4*m.nCodes
	for _, i := range cand {
		if budget < m.nCodes {
			break
		}
		budget -= m.nCodes
		off := int32(len(m.denseNext))
		m.denseNext = slices.Grow(m.denseNext, int(m.nCodes))
		for c := int32(0); c < m.nCodes; c++ {
			m.denseNext = append(m.denseNext, -1)
		}
		nd := &m.nodes[i]
		for j := nd.edgeStart; j < nd.edgeEnd; j++ {
			m.denseNext[off+int32(m.edgeCode[j])] = m.edgeNext[j]
		}
		nd.dense = off
		m.dense++
	}

	// 2-gram prefilter. A one-character word sets every pair starting with
	// its code, so the prefilter alone decides whether a word can start here.
	pairs := len(entries) * 16
	if pairs < 1<<10 {
		pairs = 1 << 10
	}
	if pairs > 1<<22 {
		pairs = 1 << 22
	}
	nbits := 1 << bits.Len(uint(pairs-1))
	m.pre = make([]uint64, nbits/64)
	m.preShift = uint32(32 - bits.TrailingZeros(uint(nbits)))
	for _, e := range entries {
		c0 := code(arena[e.off])
		if e.n == 1 {
			for c1 := uint16(0); c1 < uint16(m.nCodes); c1++ {
				h := preHash(c0, c1) >> m.preShift
				m.pre[h>>6] |= 1 << (h & 63)
			}
			continue
		}
		h := preHash(c0, code(arena[e.off+1])) >> m.preShift
		m.pre[h>>6] |= 1 << (h & 63)
	}

	// Character table: folding + class flags in one lookup.
	m.bmp = new([1 << 16]uint16)
	for r := 0; r < 1<<16; r++ {
		v := codeOf[fold[r]]
		switch cl := class[r]; {
		case cl&normalize.Ignorable != 0:
			v |= flagIgn
		case cl&normalize.Separator != 0:
			v |= flagSep
		}
		m.bmp[r] = v
	}
	m.ext = map[rune]uint16{}
	addExt := func(r rune) {
		c := code(normalize.Fold(r))
		if c == 0 {
			return
		}
		switch cl := normalize.Class(r); {
		case cl&normalize.Ignorable != 0:
			c |= flagIgn
		case cl&normalize.Separator != 0:
			c |= flagSep
		}
		m.ext[r] = c
	}
	for _, w := range words {
		for _, r := range w.Text {
			if r >= 1<<16 {
				addExt(r)
			}
		}
	}
	for _, rg := range [][2]rune{{0x1D400, 0x1D6A3}, {0x1D7CE, 0x1D7FF}, {0x1F130, 0x1F189}, {0x1F1E6, 0x1F1FF}} {
		for r := rg[0]; r <= rg[1]; r++ {
			addExt(r)
		}
	}
	return m, nil
}

func preHash(c1, c2 uint16) uint32 {
	return (uint32(c1)<<16 | uint32(c2)) * 0x9E3779B1
}

// lookupExt returns code|flags for a rune outside the BMP.
func (m *Matcher) lookupExt(r rune) uint16 {
	if v, ok := m.ext[r]; ok {
		return v
	}
	if r >= 0x1F000 && r <= 0x1FAFF { // emoji blocks
		return flagSep
	}
	switch cl := normalize.Class(r); {
	case cl&normalize.Ignorable != 0:
		return flagIgn
	case cl&normalize.Separator != 0:
		return flagSep
	}
	return 0
}

// next returns the child of s on code c, or -1. Small enough to be inlined.
func (m *Matcher) next(s int32, c uint16) int32 {
	nd := &m.nodes[s]
	if nd.dense >= 0 {
		return m.denseNext[nd.dense+int32(c)]
	}
	return m.nextSparse(nd, c)
}

func (m *Matcher) nextSparse(nd *node, c uint16) int32 {
	lo, hi := nd.edgeStart, nd.edgeEnd
	codes := m.edgeCode
	if hi-lo <= 8 {
		for j := lo; j < hi; j++ {
			if codes[j] == c {
				return m.edgeNext[j]
			}
			if codes[j] > c {
				break
			}
		}
		return -1
	}
	for lo < hi {
		mid := int32(uint32(lo+hi) >> 1)
		if codes[mid] < c {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < nd.edgeEnd && codes[lo] == c {
		return m.edgeNext[lo]
	}
	return -1
}

// Scan reports every match in text (overlapping matches included) in order
// of end position, longest first among matches that end at the same
// position. fn returning false stops the scan. Scan allocates nothing.
func (m *Matcher) Scan(text string, opt Options, fn func(Hit) bool) {
	if len(text) == 0 || len(m.words) == 0 {
		return
	}
	// The ring buffer of consumed-character offsets lives on the stack; it
	// must hold at least maxDepth entries. Most dictionaries fit in 64.
	if m.maxDepth < 64 {
		var ring [64]uint32
		m.scan(text, opt, fn, ring[:])
		return
	}
	var ring [256]uint32
	m.scan(text, opt, fn, ring[:])
}

func (m *Matcher) scan(text string, opt Options, fn func(Hit) bool, ring []uint32) {
	n := len(text)
	ringMask := uint32(len(ring) - 1)
	nodes := m.nodes
	bmp := m.bmp
	root := m.denseNext[:m.nCodes]
	gapMode := opt.MaxGap > 0 || opt.CollapseRepeats
	usePre := !gapMode
	mask := opt.Mask
	var (
		state    int32
		i, off   int
		gap      int
		prevCode uint16
		cnt      uint32 // number of consumed characters (ring writes)
	)
	r, w := utf8.DecodeRuneInString(text)
	var v uint16
	if r < 1<<16 {
		v = bmp[uint16(r)]
	} else {
		v = m.lookupExt(r)
	}
	for {
		nextOff := off + w
		var (
			vn uint16
			wn int
		)
		if nextOff < n {
			var rn rune
			rn, wn = utf8.DecodeRuneInString(text[nextOff:])
			if rn < 1<<16 {
				vn = bmp[uint16(rn)]
			} else {
				vn = m.lookupExt(rn)
			}
		}
		if state == 0 && usePre {
			// Fast path: nothing is being matched and no word can start
			// here (2-gram prefilter), so skip all bookkeeping.
			c := v & codeMask
			if c == 0 || (vn&flagIgn == 0 && !m.preTest(c, vn&codeMask)) {
				if nextOff >= n {
					return
				}
				i++
				off = nextOff
				w, v = wn, vn
				continue
			}
		}
		if v&flagIgn == 0 {
			c := v & codeMask
			prev := state
			matched := false
			if state != 0 {
				for {
					nd := &nodes[state]
					var nx int32
					if nd.dense >= 0 {
						nx = m.denseNext[nd.dense+int32(c)]
					} else {
						nx = m.nextSparse(nd, c)
					}
					if nx >= 0 {
						state = nx
						matched = true
						break
					}
					state = nd.fail
					if state == 0 {
						break
					}
				}
			}
			if !matched && c != 0 && (!usePre || vn&flagIgn != 0 || m.preTest(c, vn&codeMask)) {
				if nx := root[c]; nx >= 0 {
					state = nx
					matched = true
				}
			}
			if matched {
				ring[cnt&ringMask] = uint32(off)
				cnt++
				gap = 0
				prevCode = c
				for x := state; x >= 0; {
					nd := &nodes[x]
					if nd.word >= 0 && (mask == 0 || m.words[nd.word].Payload&mask != 0) {
						sb := int(ring[(cnt-uint32(nd.depth))&ringMask])
						if !fn(Hit{
							Word:      nd.word,
							StartRune: i + 1 - utf8.RuneCountInString(text[sb:nextOff]),
							EndRune:   i + 1,
							StartByte: sb,
							EndByte:   nextOff,
						}) {
							return
						}
					}
					x = nd.out
				}
			} else if gapMode && prev != 0 && opt.CollapseRepeats && c != 0 && c == prevCode {
				state = prev // repeated character: absorbed, no gap budget used
				gap = 0
			} else if gapMode && prev != 0 && v&flagSep != 0 && gap < opt.MaxGap {
				state = prev // separator inside a word: skipped
				gap++
			} else {
				state = 0
				gap = 0
			}
		}
		if nextOff >= n {
			return
		}
		i++
		off = nextOff
		w, v = wn, vn
	}
}

func (m *Matcher) preTest(c1, c2 uint16) bool {
	h := preHash(c1, c2) >> m.preShift
	return m.pre[h>>6]&(1<<(h&63)) != 0
}

// Words returns the compiled words; index i corresponds to Hit.Word == i.
// The slice must not be modified.
func (m *Matcher) Words() []Word { return m.words }

// Word returns word i.
func (m *Matcher) Word(i int32) Word { return m.words[i] }

// Stats returns size information.
func (m *Matcher) Stats() Stats {
	return Stats{
		Words:      len(m.words),
		Nodes:      len(m.nodes),
		Edges:      len(m.edgeCode),
		DenseNodes: m.dense,
		Alphabet:   int(m.nCodes) - 1,
		MaxLen:     m.maxDepth,
		Bytes: 1<<17 + len(m.nodes)*28 + len(m.edgeCode)*2 + len(m.edgeNext)*4 +
			len(m.denseNext)*4 + len(m.pre)*8 + len(m.ext)*16,
	}
}
