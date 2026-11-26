package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// maxInterpreterBytes bounds the interpreter tar. The tar travels in the
// workflow start event, next to the spec.
const maxInterpreterBytes = 1 << 20

// lastScheduleTimestampEnv is the only run input environ key that a
// declarative run applies. The cron trigger sets it.
const lastScheduleTimestampEnv = "LAST_SCHEDULE_TIMESTAMP"

// startError is a start failure that the user can fix. Run shows its message
// on the ExecuteWorkflow step and fails the run.
type startError struct {
	msg string
}

func (e *startError) Error() string { return e.msg }

// declarativeStart returns the workflow inputs of a DECLARATIVE run: the
// pinned interpreter tar, the interpreter arguments, and the environment.
// It reads the pin from the source pipeline and ignores uniflow_tar.
func (a *ExecuteWorkflowActor) declarativeStart(ctx context.Context, run *v2.PipelineRun) ([]byte, []interface{}, map[string]interface{}, error) {
	manifest := run.Status.SourcePipeline.Pipeline.Spec.Manifest
	pin := manifest.GetInterpreterPin()
	if pin.GetArtifact() == "" {
		return nil, nil, nil, &startError{msg: "declarative pipeline run has no interpreter pin; re-apply the pipeline through the API"}
	}
	tar, err := a.blobStore.Get(ctx, pin.Artifact)
	if err != nil {
		a.logger.Error("fetch declarative interpreter",
			zap.String("pipelineRun", run.Namespace+"/"+run.Name),
			zap.String("artifact", pin.Artifact),
			zap.Error(err))
		return nil, nil, nil, &startError{msg: fmt.Sprintf("cannot fetch interpreter %s from %s", pin.Version, pin.Artifact)}
	}
	if len(tar) > maxInterpreterBytes {
		return nil, nil, nil, &startError{msg: fmt.Sprintf("interpreter %s is %d bytes; the limit is %d", pin.Version, len(tar), maxInterpreterBytes)}
	}
	sum := sha256.Sum256(tar)
	if digest := "sha256:" + hex.EncodeToString(sum[:]); digest != pin.Digest {
		return nil, nil, nil, &startError{msg: fmt.Sprintf("interpreter %s digest mismatch: pin %s, fetched %s from %s", pin.Version, pin.Digest, digest, pin.Artifact)}
	}
	w, err := declarative.Decode(manifest)
	if err != nil {
		return nil, nil, nil, &startError{msg: "cannot decode declarative spec: " + err.Error()}
	}
	args, err := declarative.WorkflowArgs(w)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build declarative workflow args: %w", err)
	}

	envs := baseEnv(run)
	if v, ok := run.Spec.GetInput().GetFields()["environ"].GetStructValue().GetFields()[lastScheduleTimestampEnv]; ok {
		envs[lastScheduleTimestampEnv] = v.GetStringValue()
	}
	addTaskImageToEnv(run, envs)
	if err := a.addTaskCacheEnv(ctx, run, envs); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to add task cache env: %w", err)
	}
	return tar, args, envs, nil
}
