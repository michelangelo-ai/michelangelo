package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/michelangelo-ai/michelangelo/go/components/pipeline/declarative"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// maxInterpreterBytes bounds the interpreter tar. The tar travels in the
// workflow start event, next to the spec.
const maxInterpreterBytes = 1 << 20

// startError is a start failure that the user can fix. Run shows stepMessage
// on the ExecuteWorkflow step, fails the run, and logs stepMessage with
// logFields.
type startError struct {
	stepMessage string
	logFields   []zap.Field
}

func (e *startError) Error() string { return e.stepMessage }

// declarativeStart returns the workflow inputs of a DECLARATIVE run: the
// pinned interpreter tar, the interpreter arguments, and the environment.
// It reads the pin from the source pipeline and ignores uniflow_tar.
func (a *ExecuteWorkflowActor) declarativeStart(ctx context.Context, run *v2.PipelineRun) ([]byte, []interface{}, map[string]interface{}, error) {
	manifest := run.Status.SourcePipeline.Pipeline.Spec.Manifest
	pin := manifest.GetInterpreterPin()
	if pin.GetArtifact() == "" {
		return nil, nil, nil, &startError{stepMessage: "declarative pipeline run has no interpreter pin; re-apply the pipeline through the API"}
	}
	if err := declarative.CheckRunInput(run.Spec.GetInput()); err != nil {
		return nil, nil, nil, &startError{stepMessage: err.Error()}
	}
	w, err := declarative.Decode(manifest)
	if err != nil {
		return nil, nil, nil, &startError{stepMessage: "cannot decode declarative spec: " + err.Error()}
	}
	args, err := declarative.WorkflowArgs(w)
	var declErr *declarative.Error
	if errors.As(err, &declErr) {
		return nil, nil, nil, &startError{stepMessage: fmt.Sprintf("parameter %s is required; parameter overrides are not supported yet", declErr.Param)}
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build declarative workflow args: %w", err)
	}

	envs := baseEnv(run)
	if v, ok := run.Spec.GetInput().GetFields()["environ"].GetStructValue().GetFields()[declarative.LastScheduleTimestampEnv]; ok {
		envs[declarative.LastScheduleTimestampEnv] = v.GetStringValue()
	}
	addTaskImageToEnv(run, envs)
	if err := a.addTaskCacheEnv(ctx, run, envs); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to add task cache env: %w", err)
	}
	tar, err := a.fetchInterpreter(ctx, pin)
	if err != nil {
		return nil, nil, nil, err
	}
	return tar, args, envs, nil
}

// fetchInterpreter returns the pinned interpreter tar. Every error is a
// *startError, which fails the run, so declarativeStart calls it last.
func (a *ExecuteWorkflowActor) fetchInterpreter(ctx context.Context, pin *v2.InterpreterPin) ([]byte, error) {
	tar, err := a.blobStore.GetLimited(ctx, pin.Artifact, maxInterpreterBytes)
	if err != nil {
		return nil, &startError{
			stepMessage: fmt.Sprintf("cannot fetch interpreter %s", pin.Version),
			logFields:   []zap.Field{zap.String("artifact", pin.Artifact), zap.Error(err)},
		}
	}
	if len(tar) > maxInterpreterBytes {
		return nil, &startError{
			stepMessage: fmt.Sprintf("interpreter %s is larger than the 1 MiB limit", pin.Version),
			logFields:   []zap.Field{zap.String("artifact", pin.Artifact), zap.Int64("bytes", int64(len(tar)))},
		}
	}
	sum := sha256.Sum256(tar)
	if digest := "sha256:" + hex.EncodeToString(sum[:]); digest != pin.Digest {
		return nil, &startError{
			stepMessage: fmt.Sprintf("interpreter %s does not match the digest in its pin", pin.Version),
			logFields: []zap.Field{
				zap.String("artifact", pin.Artifact),
				zap.String("pinDigest", pin.Digest),
				zap.String("fetchedDigest", digest),
			},
		}
	}
	return tar, nil
}
