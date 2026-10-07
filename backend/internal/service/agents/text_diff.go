package agents

type TextChangePreview struct {
	PrefixContext  string `json:"prefix_context"`
	Original       string `json:"original"`
	Candidate      string `json:"candidate"`
	SuffixContext  string `json:"suffix_context"`
	OriginalRunes  int    `json:"original_runes"`
	CandidateRunes int    `json:"candidate_runes"`
	RemovedRunes   int    `json:"removed_runes"`
	AddedRunes     int    `json:"added_runes"`
	Changed        bool   `json:"changed"`
	Truncated      bool   `json:"truncated"`
	Preview        bool   `json:"preview"`
}

type TextDiffer struct{}

func NewTextDiffer() *TextDiffer { return &TextDiffer{} }

func (d *TextDiffer) Preview(original, candidate string, maxChangedRunes, contextRunes int) TextChangePreview {
	if maxChangedRunes < 100 || maxChangedRunes > 20000 {
		maxChangedRunes = 4000
	}
	if contextRunes < 0 || contextRunes > 500 {
		contextRunes = 120
	}
	left, right := []rune(original), []rune(candidate)
	prefix := 0
	for prefix < len(left) && prefix < len(right) && left[prefix] == right[prefix] {
		prefix++
	}
	leftEnd, rightEnd := len(left), len(right)
	for leftEnd > prefix && rightEnd > prefix && left[leftEnd-1] == right[rightEnd-1] {
		leftEnd--
		rightEnd--
	}
	prefixStart := prefix - contextRunes
	if prefixStart < 0 {
		prefixStart = 0
	}
	suffixEnd := leftEnd + contextRunes
	if candidateSuffix := rightEnd + contextRunes; candidateSuffix < suffixEnd {
		suffixEnd = candidateSuffix
	}
	commonAvailable := len(left) - leftEnd
	if len(right)-rightEnd < commonAvailable {
		commonAvailable = len(right) - rightEnd
	}
	if suffixEnd > leftEnd+commonAvailable {
		suffixEnd = leftEnd + commonAvailable
	}
	originalChanged, originalTruncated := boundedRunes(left[prefix:leftEnd], maxChangedRunes)
	candidateChanged, candidateTruncated := boundedRunes(right[prefix:rightEnd], maxChangedRunes)
	return TextChangePreview{
		PrefixContext: string(left[prefixStart:prefix]), Original: originalChanged, Candidate: candidateChanged,
		SuffixContext: string(left[leftEnd:suffixEnd]), OriginalRunes: len(left), CandidateRunes: len(right),
		RemovedRunes: leftEnd - prefix, AddedRunes: rightEnd - prefix, Changed: original != candidate,
		Truncated: originalTruncated || candidateTruncated, Preview: true,
	}
}

func boundedRunes(value []rune, limit int) (string, bool) {
	if len(value) <= limit {
		return string(value), false
	}
	return string(value[:limit]), true
}
