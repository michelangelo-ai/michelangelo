package backends

import (
	"fmt"
	"strings"
)

// ModelLoadState classifies how far a model has got on one replica of an inference server.
type ModelLoadState string

const (
	// ModelLoadStateReady means the replica has the model loaded and is serving it.
	ModelLoadStateReady ModelLoadState = "READY"
	// ModelLoadStateLoading covers every non-terminal state: the replica is not running
	// yet, has not picked the model up, or is still loading it.
	ModelLoadStateLoading ModelLoadState = "LOADING"
	// ModelLoadStateFailed means the replica tried to load the model and the load failed.
	// Waiting longer will not change the outcome without operator action.
	ModelLoadStateFailed ModelLoadState = "FAILED"
)

// ReplicaModelStatus is the load state of one model on one replica.
type ReplicaModelStatus struct {
	// Replica identifies the replica, e.g. the pod name.
	Replica string
	// Running reports whether the replica's process is up and can be asked to load models.
	Running bool
	State   ModelLoadState
	// Reason explains a LOADING or FAILED state when the backend has one.
	Reason string
}

// ModelStatus is the load state of one model across every replica of an inference server.
// The deployment controller gates traffic on Ready, which requires every desired replica
// to serve the model rather than whichever replica a Service happened to pick.
type ModelStatus struct {
	// Desired is the number of replicas the server should be running.
	Desired int32
	// Replicas holds one entry per live replica, sorted by replica name. Terminating
	// replicas are excluded because they are already leaving the Service.
	Replicas []ReplicaModelStatus
}

// Ready reports whether every desired replica is running and has the model loaded.
func (s *ModelStatus) Ready() bool {
	if s == nil || s.Desired <= 0 || int32(len(s.Replicas)) < s.Desired {
		return false
	}
	for _, replica := range s.Replicas {
		if replica.State != ModelLoadStateReady {
			return false
		}
	}
	return true
}

// Failed returns the replicas whose load failed.
func (s *ModelStatus) Failed() []ReplicaModelStatus {
	if s == nil {
		return nil
	}
	var failed []ReplicaModelStatus
	for _, replica := range s.Replicas {
		if replica.State == ModelLoadStateFailed {
			failed = append(failed, replica)
		}
	}
	return failed
}

// Replica returns the status of the named replica, if it is present.
func (s *ModelStatus) Replica(name string) (ReplicaModelStatus, bool) {
	if s == nil {
		return ReplicaModelStatus{}, false
	}
	for _, replica := range s.Replicas {
		if replica.Replica == name {
			return replica, true
		}
	}
	return ReplicaModelStatus{}, false
}

// Summary renders the status for condition messages and logs, for example
// "1/2 replicas ready (triton-x-b: LOADING model not yet loaded)".
func (s *ModelStatus) Summary() string {
	if s == nil {
		return "no status"
	}
	ready := 0
	var pending []string
	for _, replica := range s.Replicas {
		if replica.State == ModelLoadStateReady {
			ready++
			continue
		}
		detail := fmt.Sprintf("%s: %s", replica.Replica, replica.State)
		if replica.Reason != "" {
			detail += " " + replica.Reason
		}
		pending = append(pending, detail)
	}
	summary := fmt.Sprintf("%d/%d replicas ready", ready, s.Desired)
	if int32(len(s.Replicas)) < s.Desired {
		summary += fmt.Sprintf(", %d running", len(s.Replicas))
	}
	if len(pending) > 0 {
		summary += " (" + strings.Join(pending, "; ") + ")"
	}
	return summary
}
