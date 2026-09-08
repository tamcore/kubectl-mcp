package tools

import "strings"

// suggestKind finds the closest known kind to the input using Levenshtein
// distance. Returns empty string if no match is close enough (distance > 3).
func suggestKind(input string, knownKinds []string) string {
	lowerInput := strings.ToLower(input)
	bestDist := 4 // threshold: only suggest if distance <= 3
	bestKind := ""

	for _, kind := range knownKinds {
		dist := levenshtein(lowerInput, strings.ToLower(kind))
		if dist < bestDist {
			bestDist = dist
			bestKind = kind
		}
	}
	return bestKind
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row optimization.
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev = curr
	}
	return prev[lb]
}
