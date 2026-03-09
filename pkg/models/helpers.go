package models

// IsEven returns true if n is divisible by 2.
func IsEven(n int) bool {
	return n%2 == 0
}

// IsOdd returns true if n is not divisible by 2.
func IsOdd(n int) bool {
	return !IsEven(n)
}
