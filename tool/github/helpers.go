package github

func stringPtrIfNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtrIfPositive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}
