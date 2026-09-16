package model

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// GetModelsByPipelineRunRequest identifies the pipeline run whose models
	// should be returned.
	GetModelsByPipelineRunRequest struct {
		Namespace       string `json:"namespace,omitempty"`
		PipelineRunName string `json:"pipelineRunName,omitempty"`
	}

	// PipelineRunModel contains the model identity needed by downstream
	// deployment plugins.
	PipelineRunModel struct {
		Name       string `json:"name,omitempty"`
		Namespace  string `json:"namespace,omitempty"`
		RevisionID int32  `json:"revision_id"`
	}

	// GetModelsByPipelineRunResponse contains every model produced by a run.
	GetModelsByPipelineRunResponse struct {
		Models []PipelineRunModel `json:"models"`
	}
)

// GetModelsByPipelineRun returns models whose indexed source provenance
// points at the requested PipelineRun.
func (r *activities) GetModelsByPipelineRun(ctx context.Context, request *GetModelsByPipelineRunRequest) (*GetModelsByPipelineRunResponse, error) {
	if request.Namespace == "" || request.PipelineRunName == "" {
		return nil, fmt.Errorf("both \"namespace\" and \"pipeline_run_name\" are required")
	}

	response, err := r.modelService.ListModel(ctx, &v2pb.ListModelRequest{
		Namespace: request.Namespace,
		ListOptions: &metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.source_pipeline_run.name=%s", request.PipelineRunName),
		},
	})
	if err != nil {
		return nil, err
	}

	models := make([]PipelineRunModel, 0)
	if response != nil && response.ModelList != nil {
		for _, candidate := range response.ModelList.Items {
			source := candidate.Spec.GetSourcePipelineRun()
			modelNamespace := candidate.GetNamespace()
			sourceNamespace := source.GetNamespace()
			if sourceNamespace == "" {
				sourceNamespace = modelNamespace
			}
			if modelNamespace != request.Namespace || sourceNamespace != request.Namespace || source.GetName() != request.PipelineRunName {
				continue
			}
			models = append(models, PipelineRunModel{
				Name:       candidate.GetName(),
				Namespace:  candidate.GetNamespace(),
				RevisionID: candidate.Spec.GetRevisionId(),
			})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Name == models[j].Name {
			return models[i].RevisionID < models[j].RevisionID
		}
		return models[i].Name < models[j].Name
	})
	if len(models) == 0 {
		return nil, fmt.Errorf("no models found for pipeline run %s/%s", request.Namespace, request.PipelineRunName)
	}
	return &GetModelsByPipelineRunResponse{Models: models}, nil
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
