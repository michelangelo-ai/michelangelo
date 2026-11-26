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

// startError is a start failure that the user can fix. Run shows msg on the
// ExecuteWorkflow step, fails the run, and logs msg with fields. The step
// message is visible to every user of the run, so blob locations, digests, and
// sizes go only in fields.
type startError struct {
	msg    string
	fields []zap.Field
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
	// The apiserver checks the input at trigger, but a client with direct write
	// access to the PipelineRun skips that check.
	if err := declarative.CheckRunInput(run.Spec.GetInput()); err != nil {
		return nil, nil, nil, &startError{msg: err.Error()}
	}
	w, err := declarative.Decode(manifest)
	if err != nil {
		return nil, nil, nil, &startError{msg: "cannot decode declarative spec: " + err.Error()}
	}
	args, err := declarative.WorkflowArgs(w)
	var declErr *declarative.Error
	if errors.As(err, &declErr) {
		return nil, nil, nil, &startError{msg: fmt.Sprintf("parameter %s is required; parameter overrides are not supported yet", declErr.Param)}
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
	// A fetch failure is final, so the fetch runs after every retryable step.
	tar, err := a.blobStore.GetLimited(ctx, pin.Artifact, maxInterpreterBytes)
	if err != nil {
		return nil, nil, nil, &startError{
			msg:    fmt.Sprintf("cannot fetch interpreter %s", pin.Version),
			fields: []zap.Field{zap.String("artifact", pin.Artifact), zap.Error(err)},
		}
	}
	if len(tar) > maxInterpreterBytes {
		return nil, nil, nil, &startError{
			msg:    fmt.Sprintf("interpreter %s is larger than the 1 MiB limit", pin.Version),
			fields: []zap.Field{zap.String("artifact", pin.Artifact), zap.Int64("bytes", int64(len(tar)))},
		}
	}
	sum := sha256.Sum256(tar)
	if digest := "sha256:" + hex.EncodeToString(sum[:]); digest != pin.Digest {
		return nil, nil, nil, &startError{
			msg: fmt.Sprintf("interpreter %s does not match the digest in its pin", pin.Version),
			fields: []zap.Field{
				zap.String("artifact", pin.Artifact),
				zap.String("pinDigest", pin.Digest),
				zap.String("fetchedDigest", digest),
			},
		}
	}
	return tar, args, envs, nil
}
