package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/go-logr/zapr"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	"github.com/michelangelo-ai/michelangelo/go/base/zapfx"
	"github.com/michelangelo-ai/michelangelo/go/components/pipeline"
	"github.com/michelangelo-ai/michelangelo/go/components/pipelinerun"
	"github.com/michelangelo-ai/michelangelo/go/components/ray"
	"github.com/michelangelo-ai/michelangelo/go/components/spark"
	"github.com/michelangelo-ai/michelangelo/go/controllermgr"
	"github.com/michelangelo-ai/michelangelo/go/kubeproto/metrics"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/uber-go/tally"
)

const serverName = "ma-controllermgr"

// Simple metrics collector for demonstration
type MetricsCollector struct {
	mu      sync.RWMutex
	metrics map[string]float64
}

func (mc *MetricsCollector) Increment(name string, tags map[string]string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// Create a key from metric name and tags
	key := name
	for k, v := range tags {
		key += fmt.Sprintf("_%s_%s", k, v)
	}
	
	mc.metrics[key]++
}

func (mc *MetricsCollector) GetMetrics() map[string]float64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	result := make(map[string]float64)
	for k, v := range mc.metrics {
		result[k] = v
	}
	return result
}

var globalMetrics = &MetricsCollector{
	metrics: make(map[string]float64),
}

// scheme provides a Kubernetes runtime.Scheme object.
//
// This function creates a new Kubernetes runtime scheme and registers both the standard Kubernetes API types
// (via the k8s.io/client-go/kubernetes/scheme package) and custom API types defined in the proto/api/v2 package.
//
// Returns:
//   - *runtime.Scheme: A runtime scheme containing registered Kubernetes API and custom CRD types.
//   - error: An error if there is a failure during scheme registration.
func scheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := kubescheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v2pb.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func getTallyScope() (tally.Scope, error) {
	// Create basic tally scope with console output for now
	s, _ := tally.NewRootScopeWithDefaultInterval(tally.ScopeOptions{
		Prefix: serverName,
	})
	
	// Initialize the metrics collector for CRD templates
	metrics.InitializeCollector(globalMetrics)
	
	// Start Prometheus-compatible metrics endpoint
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			metricsData := globalMetrics.GetMetrics()
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			
			// Convert our metrics to Prometheus text format
			for metricName, value := range metricsData {
				// Write Prometheus format: metric_name{labels} value
				fmt.Fprintf(w, "# HELP %s CRD unmarshal error counter\n", metricName)
				fmt.Fprintf(w, "# TYPE %s counter\n", metricName)
				fmt.Fprintf(w, "%s %.0f\n", metricName, value)
			}
		})
		
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		
		fmt.Println("Starting metrics server on 0.0.0.0:8090")
		if err := http.ListenAndServe("0.0.0.0:8090", nil); err != nil {
			fmt.Printf("Failed to start metrics server: %v\n", err)
		}
	}()
	
	return s, nil
}

// options provides the FX modules and configurations used by the application.
//
// This function defines the dependencies and lifecycle management for the application by:
//   - Providing the Kubernetes runtime scheme as a dependency.
//   - Including the controllermgr.Module, which defines additional FX modules specific to the application.
//   - Setting up a logger to be used by the controller-runtime package.
//
// Returns:
//   - fx.Option: A collection of FX options defining the application's modules and configurations.
func options() fx.Option {
	return fx.Options(
		env.Module,
		zapfx.Module,
		baseconfig.Module,
		fx.Provide(scheme),
		spark.Module,
		fx.Provide(baseconfig.GetK8sConfig),
		fx.Provide(baseconfig.GetMetadataStorageConfig),
		fx.Provide(baseconfig.GetWorkflowClientConfig),
		fx.Provide(getTallyScope),
		apiHandler.CtrlMgrModule,
		ray.Module,
		cadenceclient.Module,
		pipeline.Module,
		pipelinerun.Module,
		controllermgr.Module,
		fx.Invoke(func(logger *zap.Logger) {
			ctrl.SetLogger(zapr.NewLogger(logger))
		}),
	)
}

// main initializes and runs the application.
//
// This function uses the FX framework to bootstrap the application with the provided options
// and starts the application lifecycle. The application's lifecycle will continue to run until
// an interrupt signal is received, at which point it will cleanly shut down all managed components.
func main() {

	fx.New(options()).Run()
}
