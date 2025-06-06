package tritoninferenceserver

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/deployment/provider"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"net/http"
	"time"
)

type TritonProvider struct {
	DynamicClient dynamic.Interface
}

func (r TritonProvider) GetStatus(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	//url := fmt.Sprintf("http://%s-service.%s.svc.cluster.local:8000/v2/models/%s", deployment.Name, deployment.Namespace, deployment.Spec.DesiredRevision.Name)
	url := fmt.Sprintf("http://localhost:8888/%s/%s/v2/models/%s", "bert-cola-endpoint", deployment.Name, deployment.Spec.DesiredRevision.Name)

	resp, err := http.Get(url)
	if err != nil {
		log.Error(err, "Failed to reach Triton server")
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "Failed to read response body")
		return err
	}

	var rolloutCondition *apipb.Condition
	if deployment.Status.Conditions == nil {
		deployment.Status.Conditions = make([]*apipb.Condition, 0)
		rolloutCondition = &apipb.Condition{
			Type: "DeploymentStatus",
		}
		deployment.Status.Conditions = append(deployment.Status.Conditions, rolloutCondition)
	} else {
		for _, c := range deployment.Status.Conditions {
			if c.Type == "DeploymentStatus" {
				rolloutCondition = c
			}
		}
	}

	if resp.StatusCode == http.StatusOK {
		rolloutCondition.Status = apipb.CONDITION_STATUS_TRUE
		rolloutCondition.Message = fmt.Sprintf("Triton error: %s", string(body))
		rolloutCondition.LastUpdatedTimestamp = time.Now().Unix()
		deployment.Status.State = v2pb.DEPLOYMENT_STATE_HEALTHY
		if deployment.Status.CurrentRevision == nil {
			deployment.Status.CurrentRevision = &apipb.ResourceIdentifier{
				Name:      deployment.Spec.DesiredRevision.Name,
				Namespace: deployment.Spec.DesiredRevision.Namespace,
			}
		}
	}
	return nil
}

func (r TritonProvider) Retire(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment) error {
	//TODO implement me
	panic("implement me")
}

var _ provider.Provider = &TritonProvider{}

func (r TritonProvider) Rollout(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	return r.updateEndpoint(ctx, log, deployment, model)
}

func (r TritonProvider) CreateDeployment(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	gvr := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deployment.Name,
				"namespace": deployment.Namespace,
				"labels": map[string]interface{}{
					"app": "triton-server",
				},
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "triton-server",
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":       "triton-server",
							"component": "predictor",
						},
					},
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{
								"name":  "triton",
								"image": "nvcr.io/nvidia/tritonserver:23.12-py3",
								"args": []string{
									"tritonserver",
									"--model-store=/mnt/models",
									"--grpc-port=8001",
									"--http-port=8000",
									"--allow-grpc=true",
									"--allow-http=true",
									"--allow-metrics=true",
									"--metrics-port=8002",
									"--model-control-mode=poll",
									"--repository-poll-secs=60",
									"--exit-on-error=true",
									"--log-error=true",
									"--log-warning=true",
									"--log-verbose=0",
								},
								"resources": map[string]interface{}{
									"limits":   map[string]interface{}{"cpu": "1", "memory": "2Gi"},
									"requests": map[string]interface{}{"cpu": "1", "memory": "2Gi"},
								},
								"ports": []map[string]interface{}{
									{"containerPort": 8000},
									{"containerPort": 8001},
									{"containerPort": 8002},
								},
								"volumeMounts": []map[string]interface{}{
									{"name": "workdir", "mountPath": "/mnt/models"},
								},
							},
						},
						"volumes": []map[string]interface{}{
							{"name": "workdir", "emptyDir": map[string]interface{}{}},
							{"name": "model-config", "configMap": map[string]interface{}{"name": "triton-models"}},
							{"name": "storage-secret", "secret": map[string]interface{}{"secretName": "storage-config", "items": []map[string]interface{}{{"key": "localMinIO", "path": "localMinIO.json"}}}},
						},
					},
				},
			},
		},
	}

	_, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		log.Error(err, "Failed to create Triton Deployment")
		return err
	}

	log.Info("Triton Deployment created successfully")
	return nil
}

func (r TritonProvider) updateEndpoint(ctx context.Context, log logr.Logger, deployment *v2pb.Deployment, model *v2pb.Model) error {
	gvr := schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1beta1",
		Resource: "virtualservices",
	}

	vs, err := r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Get(ctx, "bert-cola-virtualservice", metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Failed to fetch VirtualService")
		return err
	}

	httpRoutes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
	if err != nil || !found {
		log.Error(err, "Failed to get http routes from VirtualService")
		return fmt.Errorf("http routes not found")
	}

	// Dynamic prefix construction
	targetPrefix := fmt.Sprintf("/%s/%s/production", deployment.Status.CurrentRevision.Name, deployment.Name)

	for _, route := range httpRoutes {
		routeMap, ok := route.(map[string]interface{})
		if !ok {
			continue
		}

		matches, found, _ := unstructured.NestedSlice(routeMap, "match")
		if !found {
			continue
		}

		for _, match := range matches {
			matchMap, ok := match.(map[string]interface{})
			if !ok {
				continue
			}

			uriMap, found, _ := unstructured.NestedMap(matchMap, "uri")
			if !found {
				continue
			}

			if prefix, ok := uriMap["prefix"]; ok {
				prefixStr, ok := prefix.(string)
				if ok && prefixStr == targetPrefix {
					newUri := fmt.Sprintf("/v2/models/%s", deployment.Spec.DesiredRevision.Name)
					if err = unstructured.SetNestedField(routeMap, newUri, "rewrite", "uri"); err != nil {
						log.Error(err, "Failed to set rewrite uri")
						return err
					}
					break
				}
			}
		}
	}

	if err = unstructured.SetNestedSlice(vs.Object, httpRoutes, "spec", "http"); err != nil {
		log.Error(err, "Failed to update http routes in VirtualService")
		return err
	}

	_, err = r.DynamicClient.Resource(gvr).Namespace(deployment.Namespace).Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		log.Error(err, "Failed to update VirtualService")
		return err
	}

	log.Info("VirtualService updated successfully with new production route")
	return nil
}
