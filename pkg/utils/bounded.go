package utils

// BoundedLoop runs fn at most maxIter times, stopping early when fn returns
// false. Non-positive limits do not run fn.
func BoundedLoop(maxIter int, fn func() bool) {
	if maxIter <= 0 || fn == nil {
		return
	}
	for i := 0; i < maxIter; i++ {
		if !fn() {
			return
		}
	}
}
