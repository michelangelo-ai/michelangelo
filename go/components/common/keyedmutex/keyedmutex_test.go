package keyedmutex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap_LockReturnsUnlocker(t *testing.T) {
	m := New()
	unlock := m.Lock("key")
	unlock()
	// Re-locking the same key after unlock must succeed without blocking.
	unlock = m.Lock("key")
	unlock()
}

func TestMap_SameKeyReusesMutex(t *testing.T) {
	m := New()
	unlock := m.Lock("key")
	unlock()
	first := m.mutexes["key"]

	unlock = m.Lock("key")
	unlock()
	second := m.mutexes["key"]

	assert.Same(t, first, second, "repeated Lock for the same key must reuse the same mutex")
}

func TestMap_DifferentKeysIndependentMutexes(t *testing.T) {
	m := New()
	unlockA := m.Lock("a")
	// Locking a different key while "a" is held must not block.
	unlockB := m.Lock("b")
	unlockB()
	unlockA()

	assert.NotSame(t, m.mutexes["a"], m.mutexes["b"], "different keys must have independent mutexes")
}
