package goroutine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeExecute(t *testing.T) {
	panicRoutine := func() {
		panic("PANIC")
	}

	recoverFn := func(v any) {
		err, ok := v.(error)
		if !ok {
			err = fmt.Errorf("%v", v)
		}
		fmt.Println(fmt.Sprintf("Catching panic: %v", err))
	}

	// The following will not work because assert.Panics cannot handle panics occur inside another goroutine:
	// assert.Panics(t, func() { go func() { panic("PANIC") }() })
	assert.True(t, hasPanicOnGoRoutine(panicRoutine))
	assert.False(t, hasPanicOnGoRoutine(func() { SafeExecute(panicRoutine, recoverFn) }))
}

// hasPanicOnGoRoutine executes the fn inside a goroutine and returns if a panic occurs
func hasPanicOnGoRoutine(fn func()) bool {
	var wg sync.WaitGroup
	wg.Add(1)
	hasPanic := false
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				hasPanic = true
			}
		}()

		fn()
	}()

	wg.Wait()
	return hasPanic
}
