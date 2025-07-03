package oss

import (
	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/shared/gateways/inferenceserver"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Simple plugin implementations for OSS

// RolloutPlugin handles rollout operations
type RolloutPlugin struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func NewRolloutPlugin(client client.Client, gateway inferenceserver.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &RolloutPlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *RolloutPlugin) GetActors() []plugins.ConditionActor {
	// Pre-placement actors (following Uber pattern)
	prePlacementActors := []plugins.ConditionActor{
		&ValidationActor{client: p.client, logger: p.logger},
		&AssetPreparationActor{client: p.client, gateway: p.gateway, logger: p.logger},
		&ResourceAcquisitionActor{client: p.client, logger: p.logger},
	}
	
	// Placement strategy actors (OSS rolling strategy)
	placementActors := []plugins.ConditionActor{
		&ModelSyncActor{client: p.client, logger: p.logger},
		&RollingRolloutActor{client: p.client, gateway: p.gateway, logger: p.logger},
	}
	
	// Post-placement actors
	postPlacementActors := []plugins.ConditionActor{
		&RolloutCompletionActor{client: p.client, logger: p.logger},
	}
	
	// Combine all actors in sequence
	actors := make([]plugins.ConditionActor, 0, len(prePlacementActors)+len(placementActors)+len(postPlacementActors))
	actors = append(actors, prePlacementActors...)
	actors = append(actors, placementActors...)
	actors = append(actors, postPlacementActors...)
	
	return actors
}

func (p *RolloutPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	// The v2pb.Deployment.Status.Conditions is already []*apipb.Condition
	return resource.Status.Conditions
}

func (p *RolloutPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	// Find existing condition and update it, or append new one
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	// If not found, append new condition
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// CleanupPlugin handles cleanup operations  
type CleanupPlugin struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func NewCleanupPlugin(client client.Client, gateway inferenceserver.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &CleanupPlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *CleanupPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&CleanupActor{client: p.client, logger: p.logger},
	}
}

func (p *CleanupPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *CleanupPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// RollbackPlugin handles rollback operations
type RollbackPlugin struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func NewRollbackPlugin(client client.Client, gateway inferenceserver.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &RollbackPlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *RollbackPlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&RollbackActor{client: p.client, logger: p.logger},
	}
}

func (p *RollbackPlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *RollbackPlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// SteadyStatePlugin handles steady state monitoring
type SteadyStatePlugin struct {
	client  client.Client
	gateway inferenceserver.Gateway
	logger  logr.Logger
}

func NewSteadyStatePlugin(client client.Client, gateway inferenceserver.Gateway, logger logr.Logger) plugins.ConditionsPlugin {
	return &SteadyStatePlugin{
		client:  client,
		gateway: gateway,
		logger:  logger,
	}
}

func (p *SteadyStatePlugin) GetActors() []plugins.ConditionActor {
	return []plugins.ConditionActor{
		&SteadyStateActor{client: p.client, logger: p.logger},
	}
}

func (p *SteadyStatePlugin) GetConditions(resource *v2pb.Deployment) []*apipb.Condition {
	return resource.Status.Conditions
}

func (p *SteadyStatePlugin) PutCondition(resource *v2pb.Deployment, condition *apipb.Condition) {
	for i, existingCondition := range resource.Status.Conditions {
		if existingCondition.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}