// Package swd is a fast sensitive-word detection and filtering library for
// Chinese (and mixed) text.
//
// Matching is done by an immutable Aho-Corasick automaton compiled from the
// word list (see internal/automaton). Character normalization (case, full
// width, digit styles, enclosed and mathematical letters, Latin diacritics)
// is folded into the automaton's character table, so a scan makes a single
// pass over the original text, allocates nothing and reports positions in the
// original text.
//
// An Engine is safe for concurrent use. Queries never take a lock; updates
// build a new automaton and swap it in atomically, so a word added with
// AddWord is visible to the next query.
//
// Basic use:
//
//	engine, err := swd.New()
//	if err != nil {
//		log.Fatal(err)
//	}
//	engine.Check("...")                // risk level, categories and matches
//	engine.Detect("...")               // any sensitive word?
//	engine.MatchAll("...")             // every match with position, label and risk
//	engine.ReplaceWithAsterisk("...")  // mask matches with *
//	engine.AddLabeledWord("自定义词", swd.ContrabandFraud)
//
// Words are classified in two levels. Category is the first level and holds
// ten values; Label is the second level and decides a word's category, its
// default Risk and its base confidence. Risk has four states, so a caller
// can block, queue for review, or pass.
package swd
