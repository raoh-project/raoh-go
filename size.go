package raoh

// sizeRules are the constraints on how many elements a collection holds, for
// the collections whose length length gives. Each returns the check and the
// issue its failure reports, as scalar.require takes them.
type sizeRules[C any] struct{ length func(C) int }

func listSize[T any]() sizeRules[[]T] {
	return sizeRules[[]T]{func(v []T) int { return len(v) }}
}

func dictSize[T any]() sizeRules[map[string]T] {
	return sizeRules[map[string]T]{func(v map[string]T) int { return len(v) }}
}

func (r sizeRules[C]) nonEmpty() (func(C) bool, func(C) Issue) {
	return func(v C) bool { return r.length(v) > 0 }, func(C) Issue {
		return NewIssue(CodeTooSmall).WithMessageKey(KeyTooSmallNonEmpty).WithMeta("min", 1).WithMeta("actual", 0)
	}
}

func (r sizeRules[C]) atLeast(n int) (func(C) bool, func(C) Issue) {
	return func(v C) bool { return r.length(v) >= n }, func(v C) Issue {
		return NewIssue(CodeTooSmall).WithMeta("min", n).WithMeta("actual", r.length(v))
	}
}

func (r sizeRules[C]) atMost(n int) (func(C) bool, func(C) Issue) {
	return func(v C) bool { return r.length(v) <= n }, func(v C) Issue {
		return NewIssue(CodeTooBig).WithMeta("max", n).WithMeta("actual", r.length(v))
	}
}

func (r sizeRules[C]) exactly(n int) (func(C) bool, func(C) Issue) {
	return func(v C) bool { return r.length(v) == n }, func(v C) Issue {
		return NewIssue(CodeInvalidSize).WithMeta("expected", n).WithMeta("actual", r.length(v))
	}
}
