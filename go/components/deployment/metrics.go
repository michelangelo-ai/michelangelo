package deployment

// NoOpScope is a no-op metrics scope
type NoOpScope struct{}

// NewNoOpScope creates a new no-op scope
func NewNoOpScope() *NoOpScope {
	return &NoOpScope{}
}

// ControllerMetrics represents metrics for the deployment controller
type ControllerMetrics struct {
	cleanupMetrics               *StateMachineMetrics
	rolloutMetrics               *StateMachineMetrics
	rollbackMetrics              *StateMachineMetrics
	steadyStateMetrics           *StateMachineMetrics
	terminalCounter              *NoOpCounter
	stateTransitionMetrics       *actionMetrics
	getStateMetrics              *actionMetrics
	healthCheckGateMetrics       *actionMetrics
	updateResourceMetrics        *actionMetrics
	retrieveResourceMetrics      *actionMetrics
	reconcileMetrics             *actionMetrics
	createDeploymentEventMetrics *actionMetrics
}

type actionMetrics struct {
	errorCount *NoOpCounter
	count      *NoOpCounter
	duration   *NoOpTimer
}

// StateMachineMetrics represents state machine metrics
type StateMachineMetrics struct {
	completedCount *NoOpCounter
	failedCount    *NoOpCounter
	initiatedCount *NoOpCounter
}

// NoOpCounter is a no-op counter
type NoOpCounter struct{}

// Inc does nothing
func (c *NoOpCounter) Inc(value int64) {}

// NoOpTimer is a no-op timer
type NoOpTimer struct{}

// Start returns a no-op stopwatch
func (t *NoOpTimer) Start() *NoOpStopwatch {
	return &NoOpStopwatch{}
}

// NoOpStopwatch is a no-op stopwatch
type NoOpStopwatch struct{}

// Stop does nothing
func (s *NoOpStopwatch) Stop() {}

// NewControllerMetrics creates new controller metrics
func NewControllerMetrics(scope interface{}) *ControllerMetrics {
	return &ControllerMetrics{
		cleanupMetrics: &StateMachineMetrics{
			completedCount: &NoOpCounter{},
			failedCount:    &NoOpCounter{},
			initiatedCount: &NoOpCounter{},
		},
		rolloutMetrics: &StateMachineMetrics{
			completedCount: &NoOpCounter{},
			failedCount:    &NoOpCounter{},
			initiatedCount: &NoOpCounter{},
		},
		rollbackMetrics: &StateMachineMetrics{
			completedCount: &NoOpCounter{},
			failedCount:    &NoOpCounter{},
			initiatedCount: &NoOpCounter{},
		},
		steadyStateMetrics: &StateMachineMetrics{
			completedCount: &NoOpCounter{},
			failedCount:    &NoOpCounter{},
			initiatedCount: &NoOpCounter{},
		},
		terminalCounter: &NoOpCounter{},
		stateTransitionMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		getStateMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		healthCheckGateMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		updateResourceMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		retrieveResourceMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		reconcileMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
		createDeploymentEventMetrics: &actionMetrics{
			errorCount: &NoOpCounter{},
			count:      &NoOpCounter{},
			duration:   &NoOpTimer{},
		},
	}
}