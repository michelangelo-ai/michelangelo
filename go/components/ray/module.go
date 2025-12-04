// Package ray provides Kubernetes controllers for managing Ray distributed computing resources.
//
// This package integrates with KubeRay to manage Ray clusters and jobs on Kubernetes.
// Ray is a unified framework for scaling AI and Python applications, providing distributed
// computing capabilities for machine learning workloads.
//
// Architecture:
//
// The ray package consists of two main controllers:
//   - RayCluster Controller: Manages the lifecycle of Ray cluster resources
//   - RayJob Controller: Manages Ray jobs that execute on Ray clusters
//
// Both controllers integrate with the jobs scheduling system to allocate resources across
// multiple Kubernetes clusters through federated clients.
//
// Integration:
//
//   - KubeRay: Uses KubeRay operator APIs to create and manage Ray resources
//   - Job Scheduler: Integrates with the jobs scheduler for resource allocation
//   - Federated Clusters: Supports deploying Ray resources to remote Kubernetes clusters
//
// Usage:
//
//	fx.New(
//	    ray.Module,
//	    // other modules...
//	)
package ray

import (
	"go.uber.org/fx"

	"github.com/michelangelo-ai/michelangelo/go/components/ray/cluster"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/job"
)

// Module provides Uber FX dependency injection options for Ray controllers.
//
// This module combines the cluster and job controller modules to provide complete
// Ray resource management capabilities. Both controllers work together to enable
// distributed computing workloads on Kubernetes.
var (
	Module = fx.Options(
		cluster.Module,
		job.Module,
	)
)
