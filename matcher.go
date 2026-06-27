package termnav

// This file is the shared fuzzy matcher, lifted verbatim from the
// byte-identical copies that passage (internal/fuzzy) and ssherpa
// (internal/fuzzy) each carried. It is a small, dependency-free matcher
// modeled on fzf's integer scoring: it answers whether a query matches a
// candidate as an order-preserving subsequence and, if so, with what score and
// which matched rune positions (for highlighting). Scoring is deterministic
// integer arithmetic, so results are stable and golden-testable. Positions are
// rune indices into the candidate (not byte offsets), exactly what a renderer
// needs to highlight after width-aware truncation.

import "unicode"

// Scoring constants, ported from fzf's FuzzyMatchV1. The absolute values do not
// matter; the ratios do — a boundary match outranks a mid-word match, a run of
// consecutive matches outranks a scattered one, and the first matched rune is
// weighted double so a leading match dominates.
const (
	scoreMatch        = 16
	scoreGapStart     = -3
	scoreGapExtension = -1

	bonusBoundary            = scoreMatch / 2 // 8
	bonusCamel               = bonusBoundary - 1
	bonusConsecutive         = -(scoreGapStart + scoreGapExtension)
	bonusFirstCharMultiplier = 2
)

// MinScorePerRune is the average score a match must reach per query rune to be
// considered relevant (as opposed to merely a valid subsequence). It filters
// scattered subsequence matches while keeping contiguous and word-boundary
// ones. Measured boundary/consecutive matches score ~17-26 per rune; scattered
// ones ~7-9. 12 sits cleanly between, with margin on both sides.
const MinScorePerRune = 12

// Result is a successful match: its score and the ascending matched rune
// indices in the candidate. The field name Score (not Value) is kept so the
// per-app fuzzy shims can alias this type without touching call sites.
type Result struct {
	Score     int
	Positions []int
}

// Relevant reports whether a match clears the relevance threshold for a query
// of the given rune length. An empty query is always relevant.
func Relevant(r Result, queryLen int) bool {
	return queryLen <= 0 || r.Score >= queryLen*MinScorePerRune
}

type charClass int

const (
	classWhite charClass = iota
	classNonWord
	classDigit
	classLower
	classUpper
)

func classOf(r rune) charClass {
	switch {
	case r == ' ' || r == '\t' || r == '\n' || r == '\r':
		return classWhite
	case unicode.IsDigit(r):
		return classDigit
	case unicode.IsUpper(r):
		return classUpper
	case unicode.IsLower(r):
		return classLower
	case unicode.IsLetter(r):
		// Letters without case (CJK, etc.) behave as word characters.
		return classLower
	default:
		return classNonWord
	}
}

// MatchFuzzy reports whether query matches candidate as an order-preserving
// subsequence and, if so, the score and matched rune positions. Case handling
// is smart-case: an all-lowercase query matches case-insensitively; any
// uppercase rune in the query makes the whole match case-sensitive. An empty
// query matches everything with score 0 and no positions.
func MatchFuzzy(query, candidate string) (Result, bool) {
	if query == "" {
		return Result{}, true
	}
	pattern := []rune(query)
	text := []rune(candidate)
	if len(pattern) > len(text) {
		return Result{}, false
	}
	caseSensitive := hasUpper(pattern)

	sidx, eidx := -1, -1
	pidx := 0
	for idx := 0; idx < len(text); idx++ {
		if charsEqual(text[idx], pattern[pidx], caseSensitive) {
			if sidx < 0 {
				sidx = idx
			}
			pidx++
			if pidx == len(pattern) {
				eidx = idx + 1
				break
			}
		}
	}
	if eidx < 0 {
		return Result{}, false
	}
	pidx = len(pattern) - 1
	for idx := eidx - 1; idx >= sidx; idx-- {
		if charsEqual(text[idx], pattern[pidx], caseSensitive) {
			pidx--
			if pidx < 0 {
				sidx = idx
				break
			}
		}
	}
	score, positions := calculateScore(caseSensitive, text, pattern, sidx, eidx)
	return Result{Score: score, Positions: positions}, true
}

func calculateScore(caseSensitive bool, text, pattern []rune, sidx, eidx int) (int, []int) {
	pidx := 0
	score := 0
	inGap := false
	consecutive := 0
	firstBonus := 0
	positions := make([]int, 0, len(pattern))

	prevClass := classWhite
	if sidx > 0 {
		prevClass = classOf(text[sidx-1])
	}
	for idx := sidx; idx < eidx; idx++ {
		char := text[idx]
		class := classOf(char)
		if charsEqual(char, pattern[pidx], caseSensitive) {
			positions = append(positions, idx)
			score += scoreMatch
			bonus := bonusFor(prevClass, class)
			if consecutive == 0 {
				firstBonus = bonus
			} else {
				if bonus >= bonusBoundary && bonus > firstBonus {
					firstBonus = bonus
				}
				bonus = maxInt(maxInt(bonus, firstBonus), bonusConsecutive)
			}
			if pidx == 0 {
				score += bonus * bonusFirstCharMultiplier
			} else {
				score += bonus
			}
			inGap = false
			consecutive++
			pidx++
			if pidx == len(pattern) {
				break
			}
		} else {
			if inGap {
				score += scoreGapExtension
			} else {
				score += scoreGapStart
			}
			inGap = true
			consecutive = 0
			firstBonus = 0
		}
		prevClass = class
	}
	return score, positions
}

func bonusFor(prev, cur charClass) int {
	if cur == classWhite || cur == classNonWord {
		return 0
	}
	switch {
	case prev == classWhite || prev == classNonWord:
		return bonusBoundary
	case prev == classLower && cur == classUpper:
		return bonusCamel
	case prev != classDigit && cur == classDigit:
		return bonusBoundary
	}
	return 0
}

func charsEqual(a, b rune, caseSensitive bool) bool {
	if a == b {
		return true
	}
	if caseSensitive {
		return false
	}
	return unicode.ToLower(a) == unicode.ToLower(b)
}

func hasUpper(rs []rune) bool {
	for _, r := range rs {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Matcher is the pluggable filter strategy a browser uses to decide which rows
// a query keeps and how their matched runes highlight. A nil Matcher in Options
// defaults to Fuzzy{MinScorePerRune}. Match returns ok=false to reject a row.
type Matcher interface {
	Match(query, candidate string) (Result, bool)
}

// Fuzzy is the default Matcher: fzf-style subsequence scoring gated by a
// per-rune relevance floor (the exact behavior both apps' list filters used,
// MatchFuzzy + Relevant folded into one call). A zero MinScore means "use the
// package default", so the zero value Fuzzy{} is the canonical filter.
type Fuzzy struct{ MinScore int }

func (f Fuzzy) Match(query, candidate string) (Result, bool) {
	r, ok := MatchFuzzy(query, candidate)
	if !ok {
		return Result{}, false
	}
	floor := f.MinScore
	if floor <= 0 {
		floor = MinScorePerRune
	}
	if n := len([]rune(query)); n > 0 && r.Score < n*floor {
		return Result{}, false
	}
	return r, true
}

// Substring is a case-insensitive plain Contains filter with no scoring — the
// behavior passage's directory browser used (strings.Contains on the lowered
// title). Positions are left empty (no highlight), matching that surface.
type Substring struct{}

func (Substring) Match(query, candidate string) (Result, bool) {
	if query == "" {
		return Result{}, true
	}
	if containsFold(candidate, query) {
		return Result{}, true
	}
	return Result{}, false
}

func containsFold(s, sub string) bool {
	return indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	ls, lsub := []rune(toLower(s)), []rune(toLower(sub))
	if len(lsub) == 0 {
		return 0
	}
	for i := 0; i+len(lsub) <= len(ls); i++ {
		match := true
		for j := range lsub {
			if ls[i+j] != lsub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, unicode.ToLower(r))
	}
	return string(out)
}
