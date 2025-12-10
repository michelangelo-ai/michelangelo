package deployment

import (
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"go.starlark.net/starlark"

	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	deployment "github.com/michelangelo-ai/michelangelo/go/worker/activities/deployment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ starlark.HasAttrs = (*module)(nil)
)

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
	var existingDeployment *v2pb.Deployment
	err := workflow.ExecuteActivity(ctx, deployment.Activities.GetDeployment, &v2pb.GetDeploymentRequest{
		Namespace: namespace,
		Name:      deploymentName,
	}).Get(ctx, &existingDeployment)

	if err == nil {
		// Case 1: Deployment exists - Update path.
		// Update the existing deployment with the new desired revision.
		// For OSS, we must provide the complete existing deployment object
		// (including metadata.resourceVersion) and only modify the desired revision.
		updateReq := &v2pb.UpdateDeploymentRequest{
			Deployment: &v2pb.Deployment{
				ObjectMeta: existingDeployment.ObjectMeta,
				Spec:       existingDeployment.Spec,
				Status:     existingDeployment.Status,
			},
		}
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

	// NOTE: Deployment revision tracking is commented out for OSS.
	// OSS uses revision.NewNoOpManager() which disables deployment revision creation.
	// Uncomment when revision service is enabled in OSS (see controller.go line 90).
	//
	// var latestRevisionName string
	// if err := workflow.ExecuteActivity(ctx, deployment.Activities.GetLatestDeploymentRevision,
	// 	deployment.GetLatestDeploymentRevisionRequest{
	// 		Namespace:       namespace,
	// 		DeploymentName:  deploymentName,
	// 		OldRevisionName: "",
	// 	}).Get(ctx, &latestRevisionName); err != nil {
	// 	return nil, err
	// }

	// Return deployment information.
	result := starlark.NewDict(3)
	result.SetKey(starlark.String("deployment_name"), starlark.String(deploymentName))
	result.SetKey(starlark.String("model_revision_name"), starlark.String(modelRevisionName))
	// result.SetKey(starlark.String("deployment_revision_name"), starlark.String(latestRevisionName)) // Requires revision service
	return result, nil
}

func (r *module) waitForDeployment(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace, deploymentName string
	var timeout, poll int64 = 31536000, 600 // Defaults: 1 year, 10 mins

	if err := starlark.UnpackArgs("wait_for_deployment", args, kwargs,
		"namespace", &namespace,
		"deployment_name", &deploymentName,
		"timeout?", &timeout,
		"poll?", &poll,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Set up retry policy for polling the deployment status.
	retryPolicy := workflow.RetryPolicy{
		InitialInterval:          time.Second * time.Duration(poll),
		BackoffCoefficient:       1.0,
		MaximumInterval:          time.Second * time.Duration(poll),
		ExpirationInterval:       time.Second * time.Duration(timeout),
		MaximumAttempts:          0, // Unlimited retries within timeout
		NonRetriableErrorReasons: []string{"cadenceInternal:Generic", "not-found", "internal", "invalid-argument"},
	}
	ctx = workflow.WithRetryPolicy(ctx, retryPolicy)

	var finalDeployment *v2pb.Deployment
	if err := workflow.ExecuteActivity(ctx, deployment.Activities.SensorDeployment, deployment.SensorDeploymentRequest{
		Namespace:      namespace,
		DeploymentName: deploymentName,
	}).Get(ctx, &finalDeployment); err != nil {
		return nil, err
	}

	result := starlark.NewDict(1)
	result.SetKey(starlark.String("stage"), starlark.String(finalDeployment.Status.Stage.String()))
	return result, nil
}

