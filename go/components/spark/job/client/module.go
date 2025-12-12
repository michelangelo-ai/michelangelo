package client

import (
	"github.com/michelangelo-ai/michelangelo/go/components/spark/job"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
)

var (
	// Module provides Uber FX dependency injection options for the Spark client.
	//
	// This module registers the SparkClient provider, making a configured client
	// for Spark Operator interactions available to other components through
	// dependency injection.
	//
	// The client is configured to communicate with the Spark Operator's
	// SparkApplication CRDs (sparkoperator.k8s.io/v1beta2).
	//
	// Provided:
	//   - job.Client: Configured Spark client for creating and monitoring jobs
	Module = fx.Options(
		fx.Provide(register),
	)
)

// register creates and configures a SparkClient for Spark Operator interactions.
//
// This function is invoked by Uber FX during application startup. It:
//  1. Creates a runtime scheme with Spark Operator types
//  2. Configures a REST client for sparkoperator.k8s.io/v1beta2 API group
//  3. Returns a SparkClient instance implementing the job.Client interface
//
// The function panics if scheme registration or REST client creation fails,
// as this indicates a fundamental configuration problem preventing Spark
// integration.
//
// Returns a configured job.Client for Spark Operator operations.
func register(
	env env.Context,
	mgr manager.Manager,
) job.Client {
	scheme := runtime.NewScheme()
	if err := sparkv1beta2.AddToScheme(scheme); err != nil {
		panic("failed to add scheme: " + err.Error())
	}

	config := mgr.GetConfig()
	config.ContentConfig.GroupVersion = &sparkv1beta2.SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		panic("failed to create REST client: " + err.Error())
	}
	return &SparkClient{
		K8sClient:      restClient,
		ParameterCodec: runtime.NewParameterCodec(scheme),
	}
}
