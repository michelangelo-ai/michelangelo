// Package spark provides Kubernetes controllers for managing Apache Spark jobs.
//
// This package integrates with the Spark Operator to manage Spark applications on
// Kubernetes. Apache Spark is a unified analytics engine for large-scale data processing,
// providing distributed computing capabilities for data engineering and machine learning
// workloads.
//
// Architecture:
//
// The spark package consists of two main components:
//   - SparkJob Controller: Manages the lifecycle of Spark job resources
//   - Spark Client: Interfaces with the Spark Operator for job creation and monitoring
//
// Integration:
//
//   - Spark Operator: Uses Spark Operator CRDs to create and manage Spark applications
//   - SparkApplication: Creates and monitors SparkApplication resources
//   - Kubernetes: Manages Spark driver and executor pods
//
// Usage:
//
//	fx.New(
//	    spark.Module,
//	    // other modules...
//	)
package spark

import (
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job/client"
	"go.uber.org/fx"
)

var (
	// Module provides Uber FX dependency injection options for Spark controllers.
	//
	// This module combines the client and job controller modules to provide complete
	// Spark job management capabilities. The client module provides the Spark Operator
	// client implementation, while the job module provides the SparkJob controller.
	Module = fx.Options(
		client.Module,
		job.Module,
	)
)
