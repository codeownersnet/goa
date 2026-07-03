package editfile

func levenshtein(a, b string) int {
	if a == "" || b == "" {
		if len(a) > len(b) {
			return len(a)
		}
		return len(b)
	}
	la, lb := len(a), len(b)
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[i] + 1
			ins := curr[i-1] + 1
			sub := prev[i-1] + cost
			if del < ins {
				curr[i] = del
			} else {
				curr[i] = ins
			}
			if sub < curr[i] {
				curr[i] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[la]
}
