package swd

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/kirklin/go-swd/internal/automaton"
)

// Errors returned by word management methods.
var (
	ErrEmptyWord       = errors.New("swd: empty word")
	ErrInvalidCategory = errors.New("swd: invalid category")
	ErrWordTooLong     = errors.New("swd: word longer than 255 characters")
)

// Match is one occurrence of a sensitive word in a text.
//
// StartPos and EndPos are rune indices (as in []rune(text)); ByteStart and
// ByteEnd are byte offsets into the text. The span covers the original
// characters, including any separators skipped in gap mode.
type Match struct {
	Word      string   // the dictionary word, in its original spelling
	StartPos  int      // rune index of the first character
	EndPos    int      // rune index one past the last character
	ByteStart int      // byte offset of the first character
	ByteEnd   int      // byte offset one past the last character
	Category  Category // categories of the word
}

// SensitiveWord is the former name of Match.
type SensitiveWord = Match

// Engine detects and filters sensitive words. It is safe for concurrent use:
// queries never block, and every update builds a new automaton and swaps it
// in atomically.
type Engine struct {
	matcher atomic.Pointer[automaton.Matcher]
	allow   atomic.Pointer[automaton.Matcher]

	mu       sync.Mutex // guards the fields below and serializes rebuilds
	list     []automaton.Word
	index    map[string]int32
	dead     int
	allowSet map[string]struct{}

	maxGap   int
	collapse bool
}

// SWD is the former name of Engine.
type SWD = Engine

// New creates an Engine loaded with the built-in dictionary (unless
// WithoutDefaultDict is given).
func New(opts ...Option) (*Engine, error) {
	cfg := config{defaultDict: true}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	e := &Engine{
		index:    make(map[string]int32, 1<<16),
		allowSet: map[string]struct{}{},
		maxGap:   cfg.maxGap,
		collapse: cfg.collapse,
	}
	if cfg.defaultDict {
		for _, d := range defaultDict {
			cat := d.cat
			parseLines(d.data, func(w string) { e.addLocked(w, cat) })
		}
	}
	for _, w := range sortedKeys(cfg.words) {
		cat := cfg.words[w]
		nw, err := normWord(w)
		if err != nil {
			return nil, err
		}
		if !cat.IsValid() {
			return nil, fmt.Errorf("%w: %d", ErrInvalidCategory, uint32(cat))
		}
		e.addLocked(nw, cat)
	}
	for _, w := range cfg.allow {
		if w = strings.TrimSpace(w); w != "" {
			e.allowSet[w] = struct{}{}
		}
	}
	if err := e.rebuild(); err != nil {
		return nil, err
	}
	if err := e.rebuildAllow(); err != nil {
		return nil, err
	}
	return e, nil
}

func normWord(w string) (string, error) {
	w = strings.TrimSpace(w)
	if w == "" {
		return "", ErrEmptyWord
	}
	if utf8.RuneCountInString(w) > automaton.MaxWordLen {
		return "", fmt.Errorf("%w: %q", ErrWordTooLong, w)
	}
	return w, nil
}

// addLocked records w with cat (OR-ed into an existing entry). Caller holds mu.
func (e *Engine) addLocked(w string, cat Category) {
	if i, ok := e.index[w]; ok {
		e.list[i].Payload |= uint32(cat)
		return
	}
	e.index[w] = int32(len(e.list))
	e.list = append(e.list, automaton.Word{Text: w, Payload: uint32(cat)})
}

// removeLocked deletes w. Caller holds mu.
func (e *Engine) removeLocked(w string) bool {
	i, ok := e.index[w]
	if !ok {
		return false
	}
	delete(e.index, w)
	e.list[i] = automaton.Word{}
	e.dead++
	if e.dead > 64 && e.dead > len(e.list)/4 {
		live := make([]automaton.Word, 0, len(e.list)-e.dead)
		for _, w := range e.list {
			if w.Text != "" {
				e.index[w.Text] = int32(len(live))
				live = append(live, w)
			}
		}
		e.list, e.dead = live, 0
	}
	return true
}

// rebuild compiles the word list and publishes it. Caller holds mu.
func (e *Engine) rebuild() error {
	m, err := automaton.Build(e.list)
	if err != nil {
		return err
	}
	e.matcher.Store(m)
	return nil
}

func (e *Engine) rebuildAllow() error {
	if len(e.allowSet) == 0 {
		e.allow.Store(nil)
		return nil
	}
	words := make([]automaton.Word, 0, len(e.allowSet))
	for w := range e.allowSet {
		words = append(words, automaton.Word{Text: w})
	}
	slices.SortFunc(words, func(a, b automaton.Word) int { return strings.Compare(a.Text, b.Text) })
	m, err := automaton.Build(words)
	if err != nil {
		return err
	}
	e.allow.Store(m)
	return nil
}

// ---------------------------------------------------------------- queries

func (e *Engine) options(mask Category) automaton.Options {
	return automaton.Options{Mask: uint32(mask), MaxGap: e.maxGap, CollapseRepeats: e.collapse}
}

type span struct{ start, end int }

// allowSpans returns the merged, sorted rune spans covered by allowed
// phrases, or nil when there is no allow list.
func (e *Engine) allowSpans(text string) []span {
	am := e.allow.Load()
	if am == nil {
		return nil
	}
	var spans []span
	am.Scan(text, e.options(0), func(h automaton.Hit) bool {
		spans = append(spans, span{h.StartRune, h.EndRune})
		return true
	})
	if len(spans) == 0 {
		return nil
	}
	slices.SortFunc(spans, func(a, b span) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return b.end - a.end
	})
	out := spans[:1]
	for _, s := range spans[1:] {
		last := &out[len(out)-1]
		if s.start <= last.end {
			if s.end > last.end {
				last.end = s.end
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func covered(spans []span, start, end int) bool {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].start > start }) - 1
	return i >= 0 && end <= spans[i].end
}

// each calls fn for every match of the current automaton. mask 0 = all words.
func (e *Engine) each(text string, mask Category, fn func(Match) bool) {
	m := e.matcher.Load()
	if m == nil || text == "" {
		return
	}
	spans := e.allowSpans(text)
	m.Scan(text, e.options(mask), func(h automaton.Hit) bool {
		if spans != nil && covered(spans, h.StartRune, h.EndRune) {
			return true
		}
		w := m.Word(h.Word)
		return fn(Match{
			Word:      w.Text,
			StartPos:  h.StartRune,
			EndPos:    h.EndRune,
			ByteStart: h.StartByte,
			ByteEnd:   h.EndByte,
			Category:  Category(w.Payload),
		})
	})
}

func (e *Engine) detect(text string, mask Category) bool {
	found := false
	e.each(text, mask, func(Match) bool { found = true; return false })
	return found
}

func (e *Engine) first(text string, mask Category) *Match {
	var out *Match
	e.each(text, mask, func(m Match) bool { out = &m; return false })
	return out
}

func (e *Engine) collect(text string, mask Category) []Match {
	var out []Match
	e.each(text, mask, func(m Match) bool {
		if out == nil {
			out = make([]Match, 0, 8)
		}
		out = append(out, m)
		return true
	})
	return out
}

// Detect reports whether text contains any sensitive word.
func (e *Engine) Detect(text string) bool { return e.detect(text, 0) }

// DetectIn reports whether text contains a sensitive word in any of the
// given categories. Words without a category never match.
func (e *Engine) DetectIn(text string, categories ...Category) bool {
	mask := orCategories(categories)
	return mask != 0 && e.detect(text, mask)
}

// Match returns the first match (the one that ends first; the longest one
// among matches ending at the same position), or nil.
func (e *Engine) Match(text string) *Match { return e.first(text, 0) }

// MatchIn is Match restricted to the given categories.
func (e *Engine) MatchIn(text string, categories ...Category) *Match {
	mask := orCategories(categories)
	if mask == 0 {
		return nil
	}
	return e.first(text, mask)
}

// MatchAll returns every match, overlapping matches included, ordered by end
// position (longest first among matches ending at the same position).
func (e *Engine) MatchAll(text string) []Match { return e.collect(text, 0) }

// MatchAllIn is MatchAll restricted to the given categories.
func (e *Engine) MatchAllIn(text string, categories ...Category) []Match {
	mask := orCategories(categories)
	if mask == 0 {
		return nil
	}
	return e.collect(text, mask)
}

// Matches iterates over every match without allocating a slice:
//
//	for m := range engine.Matches(text) { ... }
func (e *Engine) Matches(text string) iter.Seq[Match] {
	return func(yield func(Match) bool) { e.each(text, 0, yield) }
}

// MatchesIn is Matches restricted to the given categories.
func (e *Engine) MatchesIn(text string, categories ...Category) iter.Seq[Match] {
	mask := orCategories(categories)
	return func(yield func(Match) bool) {
		if mask != 0 {
			e.each(text, mask, yield)
		}
	}
}

// ---------------------------------------------------------------- updates

// AddWord adds a word. Adding an existing word merges the categories.
// The word is visible to queries when AddWord returns.
func (e *Engine) AddWord(word string, category Category) error {
	return e.AddWords(map[string]Category{word: category})
}

// AddWords adds many words with a single rebuild.
func (e *Engine) AddWords(words map[string]Category) error {
	type item struct {
		w string
		c Category
	}
	items := make([]item, 0, len(words))
	for w, c := range words {
		nw, err := normWord(w)
		if err != nil {
			return err
		}
		if !c.IsValid() {
			return fmt.Errorf("%w: %d", ErrInvalidCategory, uint32(c))
		}
		items = append(items, item{nw, c})
	}
	if len(items) == 0 {
		return nil
	}
	slices.SortFunc(items, func(a, b item) int { return strings.Compare(a.w, b.w) })
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, it := range items {
		e.addLocked(it.w, it.c)
	}
	return e.rebuild()
}

// RemoveWord removes a word. Removing an unknown word is not an error.
func (e *Engine) RemoveWord(word string) error {
	return e.RemoveWords([]string{word})
}

// RemoveWords removes many words with a single rebuild.
func (e *Engine) RemoveWords(words []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := false
	for _, w := range words {
		if e.removeLocked(strings.TrimSpace(w)) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return e.rebuild()
}

// Clear removes every word, including the built-in dictionary.
func (e *Engine) Clear() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list, e.index, e.dead = nil, make(map[string]int32), 0
	return e.rebuild()
}

// AddAllowWords adds phrases that suppress matches lying inside them.
func (e *Engine) AddAllowWords(words ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := false
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" {
			if _, ok := e.allowSet[w]; !ok {
				e.allowSet[w] = struct{}{}
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return e.rebuildAllow()
}

// RemoveAllowWords removes allowed phrases.
func (e *Engine) RemoveAllowWords(words ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := false
	for _, w := range words {
		w = strings.TrimSpace(w)
		if _, ok := e.allowSet[w]; ok {
			delete(e.allowSet, w)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return e.rebuildAllow()
}

// Words returns a snapshot of every word and its categories.
func (e *Engine) Words() map[string]Category {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]Category, len(e.list)-e.dead)
	for _, w := range e.list {
		if w.Text != "" {
			out[w.Text] = Category(w.Payload)
		}
	}
	return out
}

// Len returns the number of words.
func (e *Engine) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.list) - e.dead
}

// Stats describes the compiled automaton.
type Stats struct {
	Words      int // distinct words after normalization
	AllowWords int
	Nodes      int // automaton states
	Alphabet   int // distinct characters used by the words
	Bytes      int // approximate memory used by the automaton tables
}

// Stats returns size information about the current automaton.
func (e *Engine) Stats() Stats {
	var s Stats
	if m := e.matcher.Load(); m != nil {
		st := m.Stats()
		s.Words, s.Nodes, s.Alphabet, s.Bytes = st.Words, st.Nodes, st.Alphabet, st.Bytes
	}
	if am := e.allow.Load(); am != nil {
		s.AllowWords = am.Stats().Words
	}
	return s
}
