package task

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxKeywords   = 3
	MinKeywordLen = 4
)

type Result struct {
	Words       int
	Characters  int
	UniqueWords int
	Keywords    []string
}

func Analyze(text string) (Result, error) {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) == 0 {
		return Result{}, ErrNothingToAnalyze
	}

	counts := make(map[string]int, len(words))
	for _, w := range words {
		counts[w]++
	}

	candidates := make([]string, 0, len(counts))
	for w := range counts {
		if utf8.RuneCountInString(w) >= MinKeywordLen {
			candidates = append(candidates, w)
		}
	}
	slices.SortFunc(candidates, func(a, b string) int {
		if c := cmp.Compare(counts[b], counts[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})

	return Result{
		Words:       len(words),
		Characters:  utf8.RuneCountInString(text),
		UniqueWords: len(counts),
		Keywords:    slices.Clone(candidates[:min(MaxKeywords, len(candidates))]),
	}, nil
}
