package model

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cadence-workflow/starlark-worker/activity"
	"github.com/cadence-workflow/starlark-worker/workflow"
	gogotypes "github.com/gogo/protobuf/types"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var Activities = (*activities)(nil)

type (
	// activities struct encapsulates the YARPC clients for the Model service.
	activities struct {
		modelService      v2pb.ModelServiceYARPCClient
		deploymentService v2pb.DeploymentServiceYARPCClient
	}

	// ModelSearchRequest DTO for ModelSearch activity
	ModelSearchRequest struct {
		Namespace      string `json:"namespace,omitempty"`
		DeploymentName string `json:"deploymentName,omitempty"`
	}

	// ModelSearchResponse DTO for ModelSearch activity
	ModelSearchResponse struct {
		ModelName       string `json:"modelName,omitempty"`
		ModelRevisionID int32  `json:"modelRevisionId"`
		Namespace       string `json:"namespace,omitempty"`
	}

	// DeployModelRequest identifies a pipeline output and its serving target.
	DeployModelRequest struct {
		Namespace           string `json:"namespace,omitempty"`
		DeploymentName      string `json:"deploymentName,omitempty"`
		PipelineRunName     string `json:"pipelineRunName,omitempty"`
		InferenceServerName string `json:"inferenceServerName,omitempty"`
		ModelName           string `json:"modelName,omitempty"`
		Actor               string `json:"actor,omitempty"`
	}

	// DeployModelResponse contains the upserted deployment and resolved model.
	DeployModelResponse struct {
		Deployment *v2pb.Deployment `json:"deployment,omitempty"`
		ModelName  string           `json:"modelName,omitempty"`
	}

	// DeploymentSensorRequest waits for one exact desired model revision.
	DeploymentSensorRequest struct {
		Namespace      string `json:"namespace,omitempty"`
		DeploymentName string `json:"deploymentName,omitempty"`
		ModelName      string `json:"modelName,omitempty"`
	}
)

const deploymentUpdateAttempts = 3

func packedString(value string) (*gogotypes.Any, error) {
	return gogotypes.MarshalAny(&gogotypes.StringValue{Value: value})
}

func modelListOptions(namespace, pipelineRunName string) (*apipb.ListOptionsExt, error) {
	namespaceValue, err := packedString(namespace)
	if err != nil {
		return nil, fmt.Errorf("pack pipeline run namespace: %w", err)
	}
	nameValue, err := packedString(pipelineRunName)
	if err != nil {
		return nil, fmt.Errorf("pack pipeline run name: %w", err)
	}
	return &apipb.ListOptionsExt{Operation: &apipb.CriterionOperation{
		Criterion: []*apipb.Criterion{
			{
				FieldName:  "spec.source_pipeline_run.namespace",
				MatchValue: namespaceValue,
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
			},
			{
				FieldName:  "spec.source_pipeline_run.name",
				MatchValue: nameValue,
				Operator:   apipb.CRITERION_OPERATOR_EQUAL,
			},
		},
	}}, nil
}

func (r *activities) resolvePipelineModel(ctx context.Context, request *DeployModelRequest) (*v2pb.Model, error) {
	listOptionsExt, err := modelListOptions(request.Namespace, request.PipelineRunName)
	if err != nil {
		return nil, err
	}
	response, err := r.modelService.ListModel(ctx, &v2pb.ListModelRequest{
		Namespace:      request.Namespace,
		ListOptions:    &metav1.ListOptions{},
		ListOptionsExt: listOptionsExt,
	})
	if err != nil {
		return nil, fmt.Errorf("list models for pipeline run %s/%s: %w", request.Namespace, request.PipelineRunName, err)
	}
	if response == nil || response.ModelList == nil {
		return nil, fmt.Errorf("list models for pipeline run %s/%s returned an empty response", request.Namespace, request.PipelineRunName)
	}

	var matches []*v2pb.Model
	for i := range response.ModelList.Items {
		candidate := &response.ModelList.Items[i]
		source := candidate.Spec.GetSourcePipelineRun()
		sourceNamespace := source.GetNamespace()
		if sourceNamespace == "" {
			sourceNamespace = request.Namespace
		}
		if source.GetName() != request.PipelineRunName || sourceNamespace != request.Namespace {
			continue
		}
		if request.ModelName != "" && candidate.GetName() != request.ModelName {
			continue
		}
		matches = append(matches, candidate)
	}

	if len(matches) == 0 {
		modelSuffix := ""
		if request.ModelName != "" {
			modelSuffix = fmt.Sprintf(" and model %q", request.ModelName)
		}
		return nil, fmt.Errorf("no model produced by pipeline run %s/%s%s", request.Namespace, request.PipelineRunName, modelSuffix)
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, candidate := range matches {
			names = append(names, candidate.GetName())
		}
		sort.Strings(names)
		return nil, fmt.Errorf("pipeline run %s/%s produced multiple models %v; pass model_name to select one", request.Namespace, request.PipelineRunName, names)
	}
	return matches[0], nil
}

func newDeployment(request *DeployModelRequest, modelName string) *v2pb.Deployment {
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      request.DeploymentName,
			Namespace: request.Namespace,
		},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{
				Name:      modelName,
				Namespace: request.Namespace,
			},
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{
					Name:      request.InferenceServerName,
					Namespace: request.Namespace,
				},
			},
			Strategy: &v2pb.DeploymentStrategy{
				RolloutStrategy: &v2pb.DeploymentStrategy_Rolling{
					Rolling: &v2pb.RollingUpdate{},
				},
			},
		},
	}
	if request.Actor != "" {
		deployment.Spec.Owner = &v2pb.UserInfo{Name: request.Actor}
	}
	return deployment
}

func prepareExistingDeployment(existing *v2pb.Deployment, request *DeployModelRequest, modelName string) (bool, error) {
	target := existing.Spec.GetInferenceServer()
	targetNamespace := target.GetNamespace()
	if targetNamespace == "" {
		targetNamespace = request.Namespace
	}
	if target.GetName() != request.InferenceServerName || targetNamespace != request.Namespace {
		return false, fmt.Errorf("deployment %s/%s targets inference server %s/%s, not %s/%s; refusing to retarget it",
			request.Namespace, request.DeploymentName, targetNamespace, target.GetName(), request.Namespace, request.InferenceServerName)
	}

	desired := existing.Spec.GetDesiredRevision()
	desiredNamespace := desired.GetNamespace()
	if desiredNamespace == "" {
		desiredNamespace = request.Namespace
	}
	changed := desired.GetName() != modelName || desiredNamespace != request.Namespace
	if changed {
		existing.Spec.DesiredRevision = &apipb.ResourceIdentifier{Name: modelName, Namespace: request.Namespace}
	}
	if request.Actor != "" && existing.Spec.GetOwner().GetName() != request.Actor {
		existing.Spec.Owner = &v2pb.UserInfo{Name: request.Actor}
		changed = true
	}
	return changed, nil
}

func (r *activities) upsertDeployment(ctx context.Context, request *DeployModelRequest, modelName string) (*v2pb.Deployment, error) {
	for attempt := 0; attempt < deploymentUpdateAttempts; attempt++ {
		getResponse, err := r.deploymentService.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
			Namespace:  request.Namespace,
			Name:       request.DeploymentName,
			GetOptions: &metav1.GetOptions{},
		})
		if err != nil {
			if yarpcerrors.FromError(err).Code() != yarpcerrors.CodeNotFound {
				return nil, fmt.Errorf("get deployment %s/%s: %w", request.Namespace, request.DeploymentName, err)
			}
			createResponse, createErr := r.deploymentService.CreateDeployment(ctx, &v2pb.CreateDeploymentRequest{
				Deployment:    newDeployment(request, modelName),
				CreateOptions: &metav1.CreateOptions{},
			})
			if createErr != nil {
				if yarpcerrors.FromError(createErr).Code() == yarpcerrors.CodeAlreadyExists && attempt+1 < deploymentUpdateAttempts {
					continue
				}
				return nil, fmt.Errorf("create deployment %s/%s: %w", request.Namespace, request.DeploymentName, createErr)
			}
			if createResponse == nil || createResponse.Deployment == nil {
				return nil, fmt.Errorf("create deployment %s/%s returned an empty response", request.Namespace, request.DeploymentName)
			}
			return createResponse.Deployment, nil
		}
		if getResponse == nil || getResponse.Deployment == nil {
			return nil, fmt.Errorf("get deployment %s/%s returned an empty response", request.Namespace, request.DeploymentName)
		}

		changed, prepareErr := prepareExistingDeployment(getResponse.Deployment, request, modelName)
		if prepareErr != nil {
			return nil, prepareErr
		}
		if !changed {
			return getResponse.Deployment, nil
		}
		updateResponse, updateErr := r.deploymentService.UpdateDeployment(ctx, &v2pb.UpdateDeploymentRequest{
			Deployment:    getResponse.Deployment,
			UpdateOptions: &metav1.UpdateOptions{},
		})
		if updateErr != nil {
			if yarpcerrors.FromError(updateErr).Code() == yarpcerrors.CodeFailedPrecondition && attempt+1 < deploymentUpdateAttempts {
				continue
			}
			return nil, fmt.Errorf("update deployment %s/%s: %w", request.Namespace, request.DeploymentName, updateErr)
		}
		if updateResponse == nil || updateResponse.Deployment == nil {
			return nil, fmt.Errorf("update deployment %s/%s returned an empty response", request.Namespace, request.DeploymentName)
		}
		return updateResponse.Deployment, nil
	}
	return nil, fmt.Errorf("failed to create or update deployment %s/%s", request.Namespace, request.DeploymentName)
}

// DeployModel resolves the model produced by a pipeline run and upserts its Deployment.
func (r *activities) DeployModel(ctx context.Context, request *DeployModelRequest) (*DeployModelResponse, error) {
	logger := activity.GetLogger(ctx)
	if request == nil || request.Namespace == "" || request.DeploymentName == "" ||
		request.PipelineRunName == "" || request.InferenceServerName == "" {
		return nil, fmt.Errorf("namespace, deployment_name, pipeline_run_name, and inference_server_name are required")
	}
	logger.Info("deploy model activity started",
		zap.String("namespace", request.Namespace),
		zap.String("pipeline_run", request.PipelineRunName),
		zap.String("deployment", request.DeploymentName))
	model, err := r.resolvePipelineModel(ctx, request)
	if err != nil {
		return nil, err
	}
	// OSS Deployments reference the immutable Model CR by its exact metadata name.
	// Unlike the internal revision store, the OSS controller does not append revision_id.
	deployment, err := r.upsertDeployment(ctx, request, model.GetName())
	if err != nil {
		return nil, err
	}
	return &DeployModelResponse{Deployment: deployment, ModelName: model.GetName()}, nil
}

func sameRevision(revision *apipb.ResourceIdentifier, namespace, name string) bool {
	if revision == nil || revision.GetName() != name {
		return false
	}
	revisionNamespace := revision.GetNamespace()
	return revisionNamespace == "" || revisionNamespace == namespace
}

func failedDeploymentStage(stage v2pb.DeploymentStage) bool {
	switch stage {
	case v2pb.DEPLOYMENT_STAGE_ROLLOUT_FAILED,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
		v2pb.DEPLOYMENT_STAGE_CLEAN_UP_FAILED:
		return true
	default:
		return false
	}
}

// DeploymentSensor returns on success or a terminal failure and retries while pending.
func (r *activities) DeploymentSensor(ctx context.Context, request *DeploymentSensorRequest) (*v2pb.Deployment, error) {
	if request == nil || request.Namespace == "" || request.DeploymentName == "" || request.ModelName == "" {
		return nil, fmt.Errorf("namespace, deployment_name, and model_name are required")
	}
	response, err := r.deploymentService.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
		Namespace:  request.Namespace,
		Name:       request.DeploymentName,
		GetOptions: &metav1.GetOptions{},
	})
	if err != nil {
		return nil, workflow.NewCustomError(ctx, yarpcerrors.FromError(err).Code().String(), err.Error())
	}
	if response == nil || response.Deployment == nil {
		return nil, fmt.Errorf("get deployment %s/%s returned an empty response", request.Namespace, request.DeploymentName)
	}
	deployment := response.Deployment
	if failedDeploymentStage(deployment.Status.Stage) {
		return deployment, nil
	}
	if deployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE &&
		deployment.Status.State == v2pb.DEPLOYMENT_STATE_HEALTHY &&
		sameRevision(deployment.Status.CurrentRevision, request.Namespace, request.ModelName) {
		return deployment, nil
	}
	return nil, workflow.NewCustomError(ctx, "DeploymentNotReady",
		fmt.Sprintf("deployment %s/%s has state %s, stage %s, current revision %s; waiting for model %s",
			request.Namespace, request.DeploymentName, deployment.Status.State.String(), deployment.Status.Stage.String(),
			deployment.Status.GetCurrentRevision().GetName(), request.ModelName))
}

func (r *activities) ListDeployments(ctx context.Context, namespace string) (*v2pb.ListDeploymentResponse, error) {
	return r.deploymentService.ListDeployment(ctx, &v2pb.ListDeploymentRequest{
		Namespace: namespace,
	})
}

func (r *activities) GetModel(ctx context.Context, namespace string, modelName string) (*v2pb.GetModelResponse, error) {
	return r.modelService.GetModel(ctx, &v2pb.GetModelRequest{
		Name:      modelName,
		Namespace: namespace,
	})
}

// ModelSearch searches model by deployment name or v1 deployment tag
func (r *activities) ModelSearch(ctx context.Context, request *ModelSearchRequest) (*ModelSearchResponse, error) {
	logger := zap.NewNop()
	if request.Namespace == "" || request.DeploymentName == "" {
		return nil, fmt.Errorf("\"namespace\" and \"deployment name\" are required to perform model search")
	}
	logger.Info("retrieving deployment", zap.String("namespace", request.Namespace), zap.String("name", request.DeploymentName))
	deploymentRes, err := r.deploymentService.GetDeployment(ctx, &v2pb.GetDeploymentRequest{
		Name:      request.DeploymentName,
		Namespace: request.Namespace,
	})
	fmt.Printf("Deployment response for deployment name: %s is %+v\n", request.DeploymentName, deploymentRes)
	if err != nil {
		return nil, err
	}
	rev := deploymentRes.Deployment.Spec.DesiredRevision
	// strip off the model revision from revision name
	modelName := rev.Name[:strings.LastIndex(rev.Name, "-")]
	revisionID := rev.Name[strings.LastIndex(rev.Name, "-")+1:]
	modelRes, err := r.modelService.GetModel(
		ctx,
		&v2pb.GetModelRequest{Name: modelName, Namespace: rev.Namespace},
	)
	if err != nil {
		return nil, err
	}
	logger.Info("retrieved model information",
		zap.String("modelName", modelName), zap.String("revisionId", revisionID))
	revisionIDNum, err := strconv.Atoi(revisionID)
	if err != nil || revisionIDNum < 0 {
		logger.Error(
			"Cannot retrieve the model revision information from deployment!",
			zap.Error(err),
			zap.String("revisionID", revisionID),
			zap.Int("revisionIDNum", revisionIDNum),
			zap.String("rawName", rev.Name),
		)
		revisionIDNum = int(modelRes.Model.Spec.RevisionId)
	}
	logger.Info("Model Search Result:",
		zap.String("modelName", modelRes.Model.Name),
		zap.String("NameSpace", modelRes.Model.GetMetadata().Namespace),
		zap.String("revisionId", revisionID),
		zap.Int("revisionIdNum", revisionIDNum),
	)
	return &ModelSearchResponse{
		ModelName:       modelRes.Model.Name,
		ModelRevisionID: int32(revisionIDNum),
		Namespace:       modelRes.Model.GetMetadata().Namespace,
	}, nil
}
