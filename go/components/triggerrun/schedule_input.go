package triggerrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	scheduleInputEnvironmentLabel          = "pipelinerun.michelangelo/environment"
	scheduleInputPipelineManifestTypeLabel = "pipeline.michelangelo/PipelineManifestType"
)

// scheduleInputHash returns a deterministic hash of the TriggerRun data consumed by
// recurring trigger workflows. It deliberately excludes notifications, which use
// ActualNotifications as their independent drift and retry-suppression marker. It
// also excludes status, volatile metadata, and one-shot action fields so periodic
// reconciliation and pause/resume handling do not create false input drift.
func scheduleInputHash(triggerRun *v2pb.TriggerRun) (string, error) {
	normalized := normalizeScheduleInput(triggerRun)
	serialized, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal normalized schedule input: %w", err)
	}

	// encoding/json sorts map keys, so the same logical protobuf input produces
	// the same bytes regardless of Go map iteration or insertion order.
	sum := sha256.Sum256(serialized)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeScheduleInput(triggerRun *v2pb.TriggerRun) *v2pb.TriggerRun {
	normalized := scheduleWorkflowInput(triggerRun)
	normalized.Spec.Notifications = nil
	normalized.TypeMeta = metav1.TypeMeta{}
	normalized.ObjectMeta = metav1.ObjectMeta{
		Name:      triggerRun.Name,
		Namespace: triggerRun.Namespace,
		Labels:    scheduleInputLabels(triggerRun.Labels),
	}
	return normalized
}

// scheduleWorkflowInput removes controller-only state and one-shot commands from
// the TriggerRun persisted in a recurring schedule's workflow action.
func scheduleWorkflowInput(triggerRun *v2pb.TriggerRun) *v2pb.TriggerRun {
	input := triggerRun.DeepCopy()
	input.Status = v2pb.TriggerRunStatus{}
	input.Spec.Action = v2pb.TRIGGER_RUN_ACTION_NO_ACTION
	input.Spec.Kill = false
	return input
}

func scheduleInputLabels(labels map[string]string) map[string]string {
	relevant := make(map[string]string, 2)
	for _, key := range []string{
		scheduleInputEnvironmentLabel,
		scheduleInputPipelineManifestTypeLabel,
	} {
		if value, ok := labels[key]; ok {
			relevant[key] = value
		}
	}
	if len(relevant) == 0 {
		return nil
	}
	return relevant
}
