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
	Word       string   // the dictionary word, in its original spelling
	Label      Label    // second-level label, e.g. PornographicAdult
	Category   Category // first-level categories of the word
	Risk       Risk     // how this match should be handled
	Confidence uint8    // 0-100, how reliably the word indicates a violation
	StartPos   int      // rune index of the first character
	EndPos     int      // rune index one past the last character
	ByteStart  int      // byte offset of the first character
	ByteEnd    int      // byte offset one past the last character
}

// Result is the verdict for a whole text.
type Result struct {
	// Risk is the highest risk among the matches, and drives the handling
	// decision. Use Risk.Suggestion for the pass/review/block advice.
	Risk Risk
	// Categories is every first-level category the text hit.
	Categories Category
	// Label is the label of the highest-risk, highest-confidence match, and
	// Confidence is that match's confidence. They are the "primary reason"
	// the text was flagged.
	Label      Label
	Confidence uint8
	// Matches lists every hit, in the order they end in the text.
	Matches []Match
}

// Hit reports whether anything was detected at all.
func (r Result) Hit() bool { return len(r.Matches) > 0 }

// Suggestion returns "pass", "review" or "block".
func (r Result) Suggestion() string { return r.Risk.Suggestion() }

// SensitiveWord is the former name of Match.
type SensitiveWord = Match

// Engine detects and filters sensitive words. It is safe for concurrent use:
// queries never block, and every update builds a new automaton and swaps it
// in atomically.
type Engine struct {
	compiled atomic.Pointer[compiled]
	allow    atomic.Pointer[automaton.Matcher]

	mu       sync.Mutex // guards the fields below and serializes rebuilds
	list     []automaton.Word
	labels   []Label // parallel to list
	index    map[string]int32
	dead     int
	allowSet map[string]struct{}

	maxGap   int
	collapse bool
}

// compiled is an immutable automaton together with the per-word metadata the
// automaton itself does not carry. meta is indexed by automaton word id.
type compiled struct {
	m    *automaton.Matcher
	meta []wordMeta
}

type wordMeta struct {
	label Label
	risk  Risk
	conf  uint8
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
		entries, err := defaultDict()
		if err != nil {
			return nil, err
		}
		for _, en := range entries {
			e.addLocked(en.word, en.label.Category(), en.label)
		}
	}
	for _, w := range sortedKeys(cfg.words) {
		nw, err := normWord(w)
		if err != nil {
			return nil, err
		}
		cat := cfg.words[w]
		if !cat.IsValid() {
			return nil, fmt.Errorf("%w: %d", ErrInvalidCategory, uint32(cat))
		}
		e.addLocked(nw, cat, Customized)
	}
	for _, w := range sortedLabelKeys(cfg.labeled) {
		nw, err := normWord(w)
		if err != nil {
			return nil, err
		}
		l := cfg.labeled[w]
		e.addLocked(nw, l.Category(), l)
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

// addLocked records w with cat and label. Adding a word that is already
// present merges the categories and keeps whichever label carries the higher
// risk, so a word listed under two labels is reported at its worst.
func (e *Engine) addLocked(w string, cat Category, label Label) {
	if i, ok := e.index[w]; ok {
		e.list[i].Payload |= uint32(cat)
		if label.Risk() > e.labels[i].Risk() {
			e.labels[i] = label
		}
		return
	}
	e.index[w] = int32(len(e.list))
	e.list = append(e.list, automaton.Word{Text: w, Payload: uint32(cat)})
	e.labels = append(e.labels, label)
}

// removeLocked deletes w. Caller holds mu.
func (e *Engine) removeLocked(w string) bool {
	i, ok := e.index[w]
	if !ok {
		return false
	}
	delete(e.index, w)
	e.list[i] = automaton.Word{}
	e.labels[i] = LabelNone
	e.dead++
	if e.dead > 64 && e.dead > len(e.list)/4 {
		live := make([]automaton.Word, 0, len(e.list)-e.dead)
		labels := make([]Label, 0, len(e.list)-e.dead)
		for j, w := range e.list {
			if w.Text != "" {
				e.index[w.Text] = int32(len(live))
				live = append(live, w)
				labels = append(labels, e.labels[j])
			}
		}
		e.list, e.labels, e.dead = live, labels, 0
	}
	return true
}

// rebuild compiles the word list and publishes it. Caller holds mu.
func (e *Engine) rebuild() error {
	m, err := automaton.Build(e.list)
	if err != nil {
		return err
	}
	// Build merges words that fold to the same characters, so the
	// automaton's word order is its own. Map it back by text.
	byText := make(map[string]Label, len(e.list))
	for i, w := range e.list {
		if w.Text != "" {
			byText[w.Text] = e.labels[i]
		}
	}
	words := m.Words()
	meta := make([]wordMeta, len(words))
	for i, w := range words {
		l := byText[w.Text]
		meta[i] = wordMeta{
			label: l,
			risk:  l.Risk(),
			conf:  confidenceOf(l, utf8.RuneCountInString(w.Text)),
		}
	}
	e.compiled.Store(&compiled{m: m, meta: meta})
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
	c := e.compiled.Load()
	if c == nil || text == "" {
		return
	}
	spans := e.allowSpans(text)
	c.m.Scan(text, e.options(mask), func(h automaton.Hit) bool {
		if spans != nil && covered(spans, h.StartRune, h.EndRune) {
			return true
		}
		w := c.m.Word(h.Word)
		md := c.meta[h.Word]
		return fn(Match{
			Word:       w.Text,
			Label:      md.label,
			Category:   Category(w.Payload),
			Risk:       md.risk,
			Confidence: md.conf,
			StartPos:   h.StartRune,
			EndPos:     h.EndRune,
			ByteStart:  h.StartByte,
			ByteEnd:    h.EndByte,
		})
	})
}

// Check returns the verdict for a whole text: the highest risk found, every
// category hit, the label that drove the decision, and all matches.
//
//	r := engine.Check(text)
//	switch r.Suggestion() {
//	case "block":  // 高风险，直接拦截
//	case "review": // 中风险，转人工复审
//	default:       // 放行
//	}
func (e *Engine) Check(text string) Result {
	return e.check(text, 0)
}

// CheckIn is Check restricted to the given categories.
func (e *Engine) CheckIn(text string, categories ...Category) Result {
	mask := orCategories(categories)
	if mask == 0 {
		return Result{}
	}
	return e.check(text, mask)
}

func (e *Engine) check(text string, mask Category) Result {
	var r Result
	e.each(text, mask, func(m Match) bool {
		r.Matches = append(r.Matches, m)
		r.Categories |= m.Category
		if m.Risk > r.Risk || (m.Risk == r.Risk && m.Confidence > r.Confidence) {
			r.Risk, r.Label, r.Confidence = m.Risk, m.Label, m.Confidence
		}
		return true
	})
	return r
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

// AddLabeledWord adds a word under a second-level label, which decides its
// category, risk and confidence.
func (e *Engine) AddLabeledWord(word string, label Label) error {
	return e.AddLabeledWords(map[string]Label{word: label})
}

// AddLabeledWords adds many labeled words with a single rebuild.
func (e *Engine) AddLabeledWords(words map[string]Label) error {
	type item struct {
		w string
		l Label
	}
	items := make([]item, 0, len(words))
	for w, l := range words {
		nw, err := normWord(w)
		if err != nil {
			return err
		}
		items = append(items, item{nw, l})
	}
	if len(items) == 0 {
		return nil
	}
	slices.SortFunc(items, func(a, b item) int { return strings.Compare(a.w, b.w) })
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, it := range items {
		e.addLocked(it.w, it.l.Category(), it.l)
	}
	return e.rebuild()
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
		e.addLocked(it.w, it.c, Customized)
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
	if c := e.compiled.Load(); c != nil {
		st := c.m.Stats()
		s.Words, s.Nodes, s.Alphabet, s.Bytes = st.Words, st.Nodes, st.Alphabet, st.Bytes
	}
	if am := e.allow.Load(); am != nil {
		s.AllowWords = am.Stats().Words
	}
	return s
}
