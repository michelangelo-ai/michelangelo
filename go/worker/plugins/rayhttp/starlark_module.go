package rayhttp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	"go.starlark.net/starlark"
	"go.uber.org/zap"
)

// Module struct implements starlark.HasAttrs interface
var _ starlark.HasAttrs = (*module)(nil)

var poll int64 = 10

type module struct {
	attributes map[string]starlark.Value
}

func newModule() starlark.Value {
	m := &module{}
	m.attributes = map[string]starlark.Value{
		"create_job": starlark.NewBuiltin("create_job", m.createRayJob).BindReceiver(m),
	}
	return m
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return starlark.String(pluginID).Hash() }
func (r *module) Attr(name string) (starlark.Value, error) { 
	if val, ok := r.attributes[name]; ok {
		return val, nil
	}
	return nil, nil 
}
func (r *module) AttrNames() []string { 
	return ext.SortedKeys(r.attributes)
}

// createRayCluster creates a new Ray cluster via the HTTP API and waits for it to be ready.
func (r *module) createRayCluster(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(thread)
	logger := workflow.GetLogger(ctx)
	
	var spec *starlark.Dict
	if err := starlark.UnpackArgs("create_cluster", args, kwargs, "spec", &spec); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}
	
	var clusterSpec map[string]interface{}
	if err := utils.AsGo(spec, &clusterSpec); err != nil {
		logger.Error("error converting to Go", zap.Error(err))
		return nil, err
	}
	
	// Marshal the cluster spec into a proper structure
	clusterSpecBytes, err := json.Marshal(clusterSpec)
	if err != nil {
		logger.Error("error marshaling cluster spec", zap.Error(err))
		return nil, err
	}
	
	var request struct {
		ClusterSpec json.RawMessage `json:"clusterSpec"`
	}
	request.ClusterSpec = clusterSpecBytes
	
	var createResponse interface{}
	err = workflow.ExecuteActivity(ctx, "activities.rayhttp.CreateRayCluster", request).Get(ctx, &createResponse)
	if err != nil {
		logger.Error("error executing create activity", zap.Error(err))
		return nil, err
	}
	
	// Extract information needed for polling
	respMap, ok := createResponse.(map[string]interface{})
	if !ok {
		logger.Error("unexpected response type", zap.Any("response", createResponse))
		return nil, fmt.Errorf("unexpected response type")
	}
	
	object, ok := respMap["object"].(map[string]interface{})
	if !ok {
		logger.Error("missing object in response", zap.Any("response", respMap))
		return nil, fmt.Errorf("missing object in response")
	}
	
	metadata, ok := object["metadata"].(map[string]interface{})
	if !ok {
		logger.Error("missing metadata in object", zap.Any("object", object))
		return nil, fmt.Errorf("missing metadata in object")
	}
	
	name, ok := metadata["name"].(string)
	if !ok {
		logger.Error("missing name in metadata", zap.Any("metadata", metadata))
		return nil, fmt.Errorf("missing name in metadata")
	}
	
	namespace, ok := metadata["namespace"].(string)
	if !ok {
		logger.Error("missing namespace in metadata", zap.Any("metadata", metadata))
		return nil, fmt.Errorf("missing namespace in metadata")
	}
	
	// Now poll for the cluster to be ready
	sensorRequest := struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}{
		Name:      name,
		Namespace: namespace,
	}
	
	// Set up polling with retry policy
	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	
	// Monitor cluster until it's ready
	var getResponse interface{}
	var clusterStatus string
	terminalStates := map[string]bool{"READY": true, "FAILED": true, "TERMINATED": true}
	
	var printJobURL = true
	for {
		err = workflow.ExecuteActivity(sensorCtx, "activities.rayhttp.GetRayCluster", sensorRequest).Get(sensorCtx, &getResponse)
		if err != nil {
			logger.Error("error executing get activity", zap.Error(err))
			return nil, err
		}
		
		// Extract cluster status
		respMap, ok = getResponse.(map[string]interface{})
		if !ok {
			continue
		}
		
		object, ok = respMap["object"].(map[string]interface{})
		if !ok {
			continue
		}
		
		status, ok := object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		
		clusterStatus, ok = status["state"].(string)
		if !ok {
			continue
		}
		
		// Check for job URL
		jobURL, ok := status["jobURL"].(string)
		if ok && jobURL != "" && printJobURL {
			thread.Print(thread, fmt.Sprintf("rayhttp | create cluster: url=%s", jobURL))
			printJobURL = false
		}
		
		thread.Print(thread, fmt.Sprintf("rayhttp | cluster status: %s", clusterStatus))
		
		if terminalStates[clusterStatus] {
			break
		}
		
		// Sleep before next poll
		workflow.Sleep(ctx, time.Second*time.Duration(poll))
	}
	
	if clusterStatus == "FAILED" || clusterStatus == "TERMINATED" {
		return nil, fmt.Errorf("cluster ended in %s state", clusterStatus)
	}
	
	var result starlark.Value
	if err := utils.AsStar(getResponse, &result); err != nil {
		logger.Error("error converting to Starlark", zap.Error(err))
		return nil, err
	}
	
	return result, nil
}

// createRayJob creates a new Ray job via the HTTP API and waits for it to be ready.
func (r *module) createRayJob(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(thread)
	logger := workflow.GetLogger(ctx)
	
	var entrypoint string
	var rayClusterNamespace string
	var rayClusterName string
	var clusterImage string
	var headCPU, workerCPU, workerInstances int64
	var headMemory, workerMemory string
	var debugEnabled bool
	var runtimeEnv *starlark.Dict

	if err := starlark.UnpackArgs("create_job", args, kwargs,
		"entrypoint", &entrypoint,
		"ray_job_namespace?", &rayClusterNamespace,
		"ray_job_name?", &rayClusterName,
		"cluster_image?", &clusterImage,
		"head_cpu?", &headCPU,
		"head_memory?", &headMemory,
		"worker_cpu?", &workerCPU,
		"worker_memory?", &workerMemory,
		"worker_instances?", &workerInstances,
		"debug_enabled?", &debugEnabled,
		"runtime_env?", &runtimeEnv,
	); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}
	
	// Convert runtime_env if provided
	var runtimeEnvGo interface{}
	if runtimeEnv != nil {
		if err := utils.AsGo(runtimeEnv, &runtimeEnvGo); err != nil {
			logger.Error("error converting runtime_env", zap.Error(err))
			return nil, err
		}
	}
	
	// Prepare ray job request with embedded cluster specification
	rayJob := map[string]interface{}{
		"kind": "RayJob",
		"apiVersion": "ray.io/v1",
		"metadata": map[string]interface{}{
			"generateName": fmt.Sprintf("uf-rj-%v-", rayClusterName),
			"namespace": rayClusterNamespace,
		},
		"spec": map[string]interface{}{
			"entrypoint": entrypoint,
			"runtimeEnv": runtimeEnvGo,
			"shutdownAfterJobFinishes": true,
			"ttlSecondsAfterFinished": 600,
			"rayClusterSpec": map[string]interface{}{
				"rayVersion": "2.3.1",
				"head": map[string]interface{}{
					"serviceType": "ClusterIP",
					"rayStartParams": map[string]interface{}{
						"block": "true",
						"dashboard-host": "0.0.0.0",
					},
					"pod": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []map[string]interface{}{
								{
									"name": "head",
									"image": clusterImage,
									"imagePullPolicy": "Never",
									"resources": map[string]interface{}{
										"requests": map[string]interface{}{
											"cpu": fmt.Sprintf("%d", headCPU),
											"memory": headMemory,
										},
									},
								},
							},
						},
					},
				},
				"workers": []map[string]interface{}{
					{
						"minInstances": workerInstances,
						"maxInstances": workerInstances,
						"nodeType": "worker-group-1",
						"rayStartParams": map[string]interface{}{
							"block": "true",
							"dashboard-host": "0.0.0.0",
						},
						"pod": map[string]interface{}{
							"spec": map[string]interface{}{
								"restartPolicy": "Never",
								"containers": []map[string]interface{}{
									{
										"name": "worker",
										"image": clusterImage,
										"imagePullPolicy": "Never",
										"resources": map[string]interface{}{
											"requests": map[string]interface{}{
												"cpu": fmt.Sprintf("%d", workerCPU),
												"memory": workerMemory,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	
	// Marshal the rayJob into a proper structure
	rayJobBytes, err := json.Marshal(rayJob)
	if err != nil {
		logger.Error("error marshaling ray job", zap.Error(err))
		return nil, err
	}
	
	var request struct {
		RayJob json.RawMessage `json:"rayJob"`
	}
	request.RayJob = rayJobBytes
	
	var createResponse interface{}
	err = workflow.ExecuteActivity(ctx, "activities.rayhttp.CreateRayJob", request).Get(ctx, &createResponse)
	if err != nil {
		logger.Error("error executing create activity", zap.Error(err))
		return nil, err
	}
	
	// Get job name and namespace from the response for monitoring
	respMap, ok := createResponse.(map[string]interface{})
	if !ok {
		logger.Error("unexpected response type", zap.Any("response", createResponse))
		return nil, fmt.Errorf("unexpected response type")
	}
	
	object, ok := respMap["object"].(map[string]interface{})
	if !ok {
		logger.Error("missing object in response", zap.Any("response", respMap))
		return nil, fmt.Errorf("missing object in response")
	}
	
	metadata, ok := object["metadata"].(map[string]interface{})
	if !ok {
		logger.Error("missing metadata in object", zap.Any("object", object))
		return nil, fmt.Errorf("missing metadata in object")
	}
	
	name, ok := metadata["name"].(string)
	if !ok {
		logger.Error("missing name in metadata", zap.Any("metadata", metadata))
		return nil, fmt.Errorf("missing name in metadata")
	}
	
	namespace, ok := metadata["namespace"].(string)
	if !ok {
		logger.Error("missing namespace in metadata", zap.Any("metadata", metadata))
		return nil, fmt.Errorf("missing namespace in metadata")
	}
	
	// Now poll for the job to be ready
	sensorRequest := struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}{
		Name:      name,
		Namespace: namespace,
	}
	
	// Set up polling with retry policy
	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.InitialInterval = time.Second * time.Duration(poll)
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)
	
	// Monitor job until it's in a terminal state
	var getResponse interface{}
	var jobStatus string
	terminalStates := map[string]bool{"SUCCEEDED": true, "FAILED": true, "STOPPED": true}
	
	for {
		err = workflow.ExecuteActivity(sensorCtx, "activities.rayhttp.GetRayJob", sensorRequest).Get(sensorCtx, &getResponse)
		if err != nil {
			logger.Error("error executing get activity", zap.Error(err))
			return nil, err
		}
		
		// Extract job status
		respMap, ok = getResponse.(map[string]interface{})
		if !ok {
			continue
		}
		
		object, ok = respMap["object"].(map[string]interface{})
		if !ok {
			continue
		}
		
		status, ok := object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		
		jobStatus, ok = status["state"].(string)
		if !ok {
			continue
		}
		
		thread.Print(thread, fmt.Sprintf("rayhttp | job status: %s", jobStatus))
		
		if terminalStates[jobStatus] {
			break
		}
		
		// Sleep before next poll
		workflow.Sleep(ctx, time.Second*time.Duration(poll))
	}
	
	if jobStatus == "FAILED" || jobStatus == "STOPPED" {
		return nil, fmt.Errorf("job ended in %s state", jobStatus)
	}
	
	var result starlark.Value
	if err := utils.AsStar(getResponse, &result); err != nil {
		logger.Error("error converting to Starlark", zap.Error(err))
		return nil, err
	}
	
	return result, nil
}

// terminateRayCluster terminates a Ray cluster via the HTTP API.
func (r *module) terminateRayCluster(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(thread)
	logger := workflow.GetLogger(ctx)
	
	var name string
	var namespace string
	var reason string
	var terminateTypeStr string
	
	if err := starlark.UnpackArgs("terminate_cluster", args, kwargs,
		"name", &name,
		"namespace", &namespace,
		"reason", &reason,
		"terminateType", &terminateTypeStr,
	); err != nil {
		logger.Error("error unpacking args", zap.Error(err))
		return nil, err
	}
	
	request := struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Type      string `json:"type"`
		Reason    string `json:"reason"`
	}{
		Name:      name,
		Namespace: namespace,
		Type:      terminateTypeStr,
		Reason:    reason,
	}
	
	var response interface{}
	err := workflow.ExecuteActivity(ctx, "activities.rayhttp.TerminateRayCluster", request).Get(ctx, &response)
	if err != nil {
		logger.Error("error executing activity", zap.Error(err))
		return nil, err
	}
	
	// Check if termination was successful
	respMap, ok := response.(map[string]interface{})
	if !ok {
		return starlark.Bool(false), nil
	}
	
	object, ok := respMap["object"].(map[string]interface{})
	if !ok {
		return starlark.Bool(false), nil
	}
	
	status, ok := object["status"].(map[string]interface{})
	if !ok {
		return starlark.Bool(false), nil
	}
	
	state, ok := status["state"].(string)
	if !ok {
		return starlark.Bool(false), nil
	}
	
	if state == "TERMINATED" {
		return starlark.Bool(true), nil
	}
	
	return starlark.Bool(false), nil
}