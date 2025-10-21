package goroutine

// SafeExecute executes the routine and recover the panic with panicHandler
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
