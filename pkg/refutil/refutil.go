package refutil

// DerefOrTypeDefault returns the dereferenced value of a pointer, or a zero value if the pointer is nil.
func DerefOrTypeDefault[T any](s *T) T {
	if s != nil {
		return *s
	}
	return *new(T)
}
