package cascadedelete

// RetainPolicy answers whether a kind's final state must be retained in MySQL
// when it is removed by a non-apiserver delete (cascade GC, kubectl, GitOps).
// The set of retained kinds is injected at the composition root.
type RetainPolicy interface {
	// RetainOnCascade reports whether objects of the given Kubernetes Kind
	// (as resolved via scheme.ObjectKinds) should have their final state
	// retained rather than soft-deleted.
	RetainOnCascade(kind string) bool
}

// staticRetainPolicy is a RetainPolicy backed by a fixed set of kinds.
type staticRetainPolicy struct {
	kinds map[string]bool
}

// NewStaticRetainPolicy returns a RetainPolicy that retains exactly the given
// kinds, supplied by the caller (the composition root).
func NewStaticRetainPolicy(kinds ...string) RetainPolicy {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return staticRetainPolicy{kinds: set}
}

// RetainOnCascade reports whether the given kind is in the retained set.
func (p staticRetainPolicy) RetainOnCascade(kind string) bool {
	return p.kinds[kind]
}
