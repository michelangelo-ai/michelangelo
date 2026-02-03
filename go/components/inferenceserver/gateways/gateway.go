package gateways

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	backendCommon "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/common"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/clientfactory"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/configmap"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/endpointregistry"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ Gateway = &gateway{}

// gateway implements the Gateway interface
type gateway struct {
	endpointRegistry       endpointregistry.EndpointRegistry
	kubeClient             client.Client
	registry               *registry
	modelConfigMapProvider configmap.ModelConfigMapProvider
}

type Params struct {
	Logger                 *zap.Logger
	KubeClient             client.Client
	ClientFactory          clientfactory.ClientFactory
	ModelConfigMapProvider configmap.ModelConfigMapProvider
	EndpointRegistry       endpointregistry.EndpointRegistry
}

// NewGatewayWithClients creates a new inference server gateway with Kubernetes clients
func NewGatewayWithClients(p Params) Gateway {
	gateway := &gateway{
		endpointRegistry: p.EndpointRegistry,
		kubeClient:       p.KubeClient,
		registry:         newRegistry(),

		modelConfigMapProvider: p.ModelConfigMapProvider,
	}

	// Register Triton backend with its endpoint configuration
	gateway.registry.registerBackend(v2pb.BACKEND_TYPE_TRITON, backends.NewTritonBackend(p.ClientFactory, p.ModelConfigMapProvider, p.Logger))
	return gateway
}

// LoadModel initiates loading a model into an inference server
func (g *gateway) LoadModel(ctx context.Context, logger *zap.Logger, modelName string, storagePath string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection) error {
	targetClusterConnection := buildClusterTargetConnection(targetCluster)
	if err := g.modelConfigMapProvider.AddModelToConfigMap(ctx, inferenceServerName, namespace, configmap.ModelConfigEntry{
		Name:        modelName,
		StoragePath: storagePath,
	}, targetClusterConnection); err != nil {
		return fmt.Errorf("failed to initiate model loading: %w", err)
	}
	return nil
}

// UnloadModel initiates unloading a model from an inference server
func (g *gateway) UnloadModel(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection) error {
	// Currrently, the only way to unload a model is to remove it from an inference server's configmap
	targetClusterConnection := buildClusterTargetConnection(targetCluster)
	if err := g.modelConfigMapProvider.RemoveModelFromConfigMap(ctx, inferenceServerName, namespace, modelName, targetClusterConnection); err != nil {
		return fmt.Errorf("failed to initiate model unloading: %w", err)
	}
	return nil
}

// CheckModelStatus dispatches model status checking based on backend type
func (g *gateway) CheckModelStatus(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error) {
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("failed to get backend for model %s on %s/%s: %w", modelName, namespace, inferenceServerName, err)
	}
	targetClusterConnection := buildClusterTargetConnection(targetCluster)
	return backend.CheckModelStatus(ctx, modelName, inferenceServerName, namespace, targetClusterConnection)
}

// CheckModelExists checks if a model exists in an inference server.
func (g *gateway) CheckModelExists(ctx context.Context, logger *zap.Logger, modelName string, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error) {
	targetClusterConnection := buildClusterTargetConnection(targetCluster)
	currentConfigs, err := g.modelConfigMapProvider.GetModelsFromConfigMap(ctx, inferenceServerName, namespace, targetClusterConnection)
	if err != nil {
		return false, fmt.Errorf("failed to check existance of model %s in inference server %s in namespace %s: %w", modelName, inferenceServerName, namespace, err)
	}

	for _, config := range currentConfigs {
		if config.Name == modelName {
			return true, nil
		}
	}
	return false, nil
}

// IsHealthy dispatches health checking based on backend type
func (g *gateway) InferenceServerIsHealthy(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string, targetCluster *TargetClusterConnection, backendType v2pb.BackendType) (bool, error) {
	if backendType == v2pb.BACKEND_TYPE_INVALID {
		return false, fmt.Errorf("invalid backend type: %v", backendType)
	}
	backend, err := g.registry.getBackend(backendType)
	if err != nil {
		return false, fmt.Errorf("unable to get backend for inference server %s in namespace %s: %w", inferenceServerName, namespace, err)
	}
	targetClusterConnection := buildClusterTargetConnection(targetCluster)
	return backend.IsHealthy(ctx, inferenceServerName, namespace, targetClusterConnection)
}

func (g *gateway) GetDeploymentTargetInfo(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (*DeploymentTargetInfo, error) {
	// Get the inference server resource
	inferenceServer, err := g.getInferenceServer(ctx, logger, inferenceServerName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get inference server resource: %w", err)
	}

	if inferenceServer.Spec.GetDeploymentStrategy().GetControlPlaneClusterDeployment() != nil {
		return &DeploymentTargetInfo{
			BackendType: inferenceServer.Spec.BackendType,
			ClusterTargets: []*TargetClusterConnection{{
				IsControlPlaneCluster: true,
			}},
		}, nil
	}

	// Retrieve registered endpoints for multi-cluster discovery
	endpoints, err := g.endpointRegistry.ListRegisteredEndpoints(ctx, logger, inferenceServerName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list registered endpoints: %w", err)
	}

	// Filter to only include registered remote clusters
	registeredClusters := make(map[string]*v2pb.ClusterTarget)
	for _, endpoint := range endpoints {
		registeredClusters[endpoint.ClusterID] = nil
	}

	for _, clusterTarget := range inferenceServer.Spec.GetDeploymentStrategy().GetRemoteClusterDeployment().GetClusterTargets() {
		if _, ok := registeredClusters[clusterTarget.ClusterId]; !ok {
			continue
		}
		registeredClusters[clusterTarget.ClusterId] = clusterTarget
	}

	registeredClustersList := make([]*TargetClusterConnection, 0, len(registeredClusters))
	for _, clusterTarget := range registeredClusters {
		if clusterTarget == nil {
			continue
		}
		registeredClustersList = append(registeredClustersList, &TargetClusterConnection{
			ClusterId: clusterTarget.ClusterId,
			Host:      clusterTarget.GetKubernetes().GetHost(),
			Port:      clusterTarget.GetKubernetes().GetPort(),
			TokenTag:  clusterTarget.GetKubernetes().GetTokenTag(),
			CaDataTag: clusterTarget.GetKubernetes().GetCaDataTag(),
		})
	}

	return &DeploymentTargetInfo{
		BackendType:    inferenceServer.Spec.BackendType,
		ClusterTargets: registeredClustersList,
	}, nil
}

func (g *gateway) GetControlPlaneServiceName(ctx context.Context, logger *zap.Logger, inferenceServerName string, namespace string) (string, error) {
	inferenceServer, err := g.getInferenceServer(ctx, logger, inferenceServerName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get inference server resource for control plane service name: %w", err)
	}
	if inferenceServer.Spec.GetDeploymentStrategy().GetControlPlaneClusterDeployment() != nil {
		return backendCommon.GenerateInferenceServiceName(inferenceServerName), nil
	}
	return g.endpointRegistry.GetControlPlaneServiceName(inferenceServerName), nil
}

func (g *gateway) getInferenceServer(ctx context.Context, logger *zap.Logger, inferenceServerName, namespace string) (*v2pb.InferenceServer, error) {
	inferenceServer := &v2pb.InferenceServer{}
	err := g.kubeClient.Get(ctx, client.ObjectKey{
		Name:      inferenceServerName,
		Namespace: namespace,
	}, inferenceServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get inference server resource: %w", err)
	}
	return inferenceServer, nil
}

func buildClusterTargetConnection(targetCluster *TargetClusterConnection) *v2pb.ClusterTarget {
	if !targetCluster.IsControlPlaneCluster {
		return &v2pb.ClusterTarget{
			ClusterId: targetCluster.ClusterId,
			Config: &v2pb.ClusterTarget_Kubernetes{
				Kubernetes: &v2pb.ConnectionSpec{
					Host:      targetCluster.Host,
					Port:      targetCluster.Port,
					TokenTag:  targetCluster.TokenTag,
					CaDataTag: targetCluster.CaDataTag,
				},
			},
		}
	}
	return nil
}
