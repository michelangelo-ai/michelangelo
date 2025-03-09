package cadence

import (
	"github.com/michelangelo-ai/michelangelo/go/cadence-starlark/cadstar"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"log"
	"time"
)

// CreateClient initializes a new Temporal client.
func CreateClient(
	hostPort string,
	namespace string,
) client.Client {
	c, err := client.Dial(client.Options{
		HostPort:      hostPort,
		Namespace:     namespace,
		DataConverter: &cadstar.DataConverter{},
	})
	if err != nil {
		log.Fatalln("Failed to create Temporal client:", err)
	}
	return c
}

// CreateWorker initializes a new Temporal worker.
func CreateWorker(
	c client.Client,
	taskQueue string,
) worker.Worker {
	return worker.New(c, taskQueue, worker.Options{
		BackgroundActivityContext:               nil,              // ✅ Still valid
		EnableSessionWorker:                     false,            // ✅ Still valid
		EnableLoggingInReplay:                   true,             // ✅ Still valid
		MaxConcurrentActivityExecutionSize:      10,               // ✅ Still valid
		MaxConcurrentWorkflowTaskExecutionSize:  5,                // ✅ Still valid
		MaxConcurrentLocalActivityExecutionSize: 5,                // ✅ Still valid
		TaskQueueActivitiesPerSecond:            1000.0,           // ✅ Matches latest Temporal version
		WorkerActivitiesPerSecond:               500.0,            // ✅ Matches latest Temporal version
		MaxConcurrentWorkflowTaskPollers:        5,                // ✅ Temporal equivalent
		MaxConcurrentActivityTaskPollers:        5,                // ✅ Temporal equivalent
		DisableWorkflowWorker:                   false,            // ✅ New in Temporal
		LocalActivityWorkerOnly:                 false,            // ✅ New in Temporal
		Identity:                                "custom-worker",  // ✅ Optional but useful
		DeadlockDetectionTimeout:                time.Second * 10, // ✅ New in Temporal (Optional)
		DefaultHeartbeatThrottleInterval:        time.Second * 30, // ✅ New in Temporal (Optional)
		MaxHeartbeatThrottleInterval:            time.Minute * 1,  // ✅ New in Temporal (Optional)
		Interceptors:                            nil,              // ✅ Temporal now supports Interceptors
		OnFatalError:                            nil,              // ✅ Temporal supports this for handling errors
		DisableEagerActivities:                  false,            // ✅ Temporal supports eager execution
		MaxConcurrentEagerActivityExecutionSize: 10,               // ✅ Temporal now limits eager activity execution
	})
}
