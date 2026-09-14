package deployment

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"

	deployment "github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ starlark.HasAttrs = (*module)(nil)
)

// deploymentFailedStages mirrors the Python mirror's _DEPLOYMENT_FAILED_STAGES:
// terminal stages that indicate the deployment did not succeed.
var deploymentFailedStages = map[v2pb.DeploymentStage]bool{
	v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED:    true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED:   true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:   true,
	v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE: true,
	v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE: true,
}

type module struct {
	attributes map[string]starlark.Value
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_or_update_deployment": starlark.NewBuiltin("create_or_update_deployment", m.createOrUpdateDeployment).BindReceiver(m),
		"wait_for_deployment":         starlark.NewBuiltin("wait_for_deployment", m.waitForDeployment).BindReceiver(m),
	}
	return m
}

func (r *module) createOrUpdateDeployment(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace, deploymentName, modelRevisionName, deploymentTemplate string
	if err := starlark.UnpackArgs("create_or_update_deployment", args, kwargs,
		"namespace", &namespace,
		"deployment_name", &deploymentName,
		"model_revision_name", &modelRevisionName,
		"deployment_template?", &deploymentTemplate,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Check if the deployment already exists to determine if we should update or create.
	// GetDeployment reports a not-found deployment as (nil, nil), so any non-nil
	// error here is a genuine failure (transient, auth, etc.) and must not be
	// treated as "deployment doesn't exist".
	var existingDeployment *v2pb.Deployment
	if err := workflow.ExecuteActivity(ctx, deployment.Activities.GetDeployment, &v2pb.GetDeploymentRequest{
		Namespace: namespace,
		Name:      deploymentName,
	}).Get(ctx, &existingDeployment); err != nil {
		return nil, err
	}

	if existingDeployment != nil {
		// Case 1: Deployment exists - Update path.
		// Update the existing deployment with the new desired revision.
		updateReq := &v2pb.UpdateDeploymentRequest{
			Deployment: &v2pb.Deployment{
				ObjectMeta: existingDeployment.ObjectMeta,
				Spec:       existingDeployment.Spec,
			},
		}
		// Reset status: it's server-managed and must not be pushed back from a
		// stale read, mirroring the Python implementation's behavior.
		updateReq.Deployment.Status = v2pb.DeploymentStatus{}
		// Override only the desired revision with the new value.
		updateReq.Deployment.Spec.DesiredRevision = &apipb.ResourceIdentifier{
			Name:      modelRevisionName,
			Namespace: namespace,
		}
		if err := workflow.ExecuteActivity(ctx, deployment.Activities.UpdateDeployment, updateReq).Get(ctx, nil); err != nil {
			return nil, err
		}

	} else {
		// Case 2: Deployment does not exist - Create path.
		// We will clone from the provided template.
		if deploymentTemplate == "" {
			return nil, fmt.Errorf("deployment_template required")
		}

		// Retrieve the template deployment to use as a base.
		var template *v2pb.Deployment
		if err := workflow.ExecuteActivity(ctx, deployment.Activities.GetDeployment, &v2pb.GetDeploymentRequest{
			Namespace: namespace,
			Name:      deploymentTemplate,
		}).Get(ctx, &template); err != nil {
			return nil, err
		}
		if template == nil {
			return nil, fmt.Errorf("deployment_template %q not found in namespace %q", deploymentTemplate, namespace)
		}

		// Create a new deployment object by copying the template and applying modifications.
		newDeployment := &v2pb.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deploymentName,
				Namespace: namespace,
				Labels:    template.Labels,
			},
			Spec: template.Spec,
		}
		newDeployment.Spec.DesiredRevision = &apipb.ResourceIdentifier{
			Name:      modelRevisionName,
			Namespace: namespace,
		}
		newDeployment.Status = v2pb.DeploymentStatus{} // Reset status for the new deployment.

		// Execute the creation activity.
		if err := workflow.ExecuteActivity(ctx, deployment.Activities.CreateDeployment, &v2pb.CreateDeploymentRequest{
			Deployment: newDeployment,
		}).Get(ctx, nil); err != nil {
			return nil, err
		}
	}

	// Return deployment information.
	result := starlark.NewDict(2)
	result.SetKey(starlark.String("deployment_name"), starlark.String(deploymentName))
	result.SetKey(starlark.String("model_revision_name"), starlark.String(modelRevisionName))
	return result, nil
}

func (r *module) waitForDeployment(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace, deploymentName, expectedModelRevision string
	var timeout, poll int64 = 0, 600 // Defaults: LongTimeout (below), 10 mins

	if err := starlark.UnpackArgs("wait_for_deployment", args, kwargs,
		"namespace", &namespace,
		"deployment_name", &deploymentName,
		"expected_model_revision_name", &expectedModelRevision,
		"timeout?", &timeout,
		"poll?", &poll,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}
	if timeout == 0 {
		timeout = int64(utils.LongTimeout.Seconds())
	}

	// Set up retry policy for polling the deployment status, following the
	// same shared sensor-retry convention as the other worker plugins.
	srp := utils.DefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Second * time.Duration(timeout)
	srp.InitialInterval = time.Second * time.Duration(poll)
	ctx = workflow.WithRetryPolicy(ctx, srp)

	var finalDeployment *v2pb.Deployment
	if err := workflow.ExecuteActivity(ctx, deployment.Activities.SensorDeployment, deployment.SensorDeploymentRequest{
		Namespace:             namespace,
		DeploymentName:        deploymentName,
		ExpectedModelRevision: expectedModelRevision,
	}).Get(ctx, &finalDeployment); err != nil {
		return nil, err
	}

	// Extract revision information
	currentRev := ""
	if finalDeployment.Status.GetCurrentRevision() != nil {
		currentRev = finalDeployment.Status.GetCurrentRevision().GetName()
	}
	desiredRev := ""
	if finalDeployment.Spec.GetDesiredRevision() != nil {
		desiredRev = finalDeployment.Spec.GetDesiredRevision().GetName()
	}

	// SensorDeployment reports any terminal stage as success and defers the
	// success/failure distinction to the caller; make that check here so a
	// failed rollout is reported as an error, matching the Python mirror.
	if deploymentFailedStages[finalDeployment.Status.Stage] {
		return nil, fmt.Errorf("deployment failed with stage: %s", finalDeployment.Status.Stage.String())
	}

	result := starlark.NewDict(3)
	result.SetKey(starlark.String("stage"), starlark.String(finalDeployment.Status.Stage.String()))
	result.SetKey(starlark.String("current_revision"), starlark.String(currentRev))
	result.SetKey(starlark.String("desired_revision"), starlark.String(desiredRev))
	return result, nil
}
