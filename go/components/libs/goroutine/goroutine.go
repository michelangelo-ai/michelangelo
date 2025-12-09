// Package goroutine provides utilities for safe goroutine execution with panic recovery.
// It helps prevent goroutine panics from crashing the entire application by providing
// graceful panic handling mechanisms.
package goroutine

// SafeExecute executes a function in a new goroutine with panic recovery.
// If the routine panics, the panic is recovered and passed to the recoverFn handler
// instead of crashing the application.
//
// This function is useful for:
//   - Background tasks where failures should be logged but not crash the app
//   - Event handlers that process user input
//   - Workers in a goroutine pool
//   - Any goroutine where panics should be gracefully handled
//
// Parameters:
//   - routine: The function to execute in a goroutine. Should contain the main work.
//   - recoverFn: Handler called if routine panics. Receives the panic value.
//     Should log or handle the panic appropriately.
//
// Example:
//
//	goroutine.SafeExecute(
//	    func() { doWork() },
//	    func(err any) { logger.Error("worker panicked", zap.Any("error", err)) },
//	)
func SafeExecute(routine func(), recoverFn func(any)) {
	go func() {
		defer handlePanic(recoverFn)
		routine()
	}()
}

func handlePanic(recoverFn func(any)) {
	if r := recover(); r != nil {
		recoverFn(r)
	}
}
