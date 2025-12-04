// Package kuberay provides Kubernetes client support for KubeRay resources.
//
// This package integrates with the KubeRay operator by providing client-side
// type definitions, scheme registration, and REST client construction for
// interacting with Ray custom resources (RayCluster, RayJob, etc.).
//
// KubeRay Integration:
//
// KubeRay is the Kubernetes operator for Ray, providing CRDs for managing
// Ray clusters and jobs. This package enables programmatic access to these
// resources from Go code.
//
// Components:
//
//   - REST Client: Provides a configured REST client for ray.io/v1 API group
//   - Scheme Registration: Registers KubeRay types with runtime scheme
//   - Type Definitions: Imports and registers RayCluster and RayClusterList
//
// Usage:
//
// The package automatically registers KubeRay types during initialization and
// provides a REST client factory through the FX module for dependency injection.
//
// +groupName=ray.io
package kuberay
