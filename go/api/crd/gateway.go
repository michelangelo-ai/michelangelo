//go:generate mamockgen Gateway
package crd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"

	"go.uber.org/fx"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Gateway is the interface for interacting with k8s CRDs.
type Gateway interface {
	// ConditionalUpsert creates or updates CRD, also checks for CRD compatibility before update
	ConditionalUpsert(ctx context.Context, crd *apiextv1.CustomResourceDefinition, enableIncompatibleUpdate bool) error

	// Delete CRDs on server, also check for instance of the CRD before deletion
	Delete(ctx context.Context, crdToDelete *apiextv1.CustomResourceDefinition) error

	// List get all CRDs on server
	List(ctx context.Context) (*apiextv1.CustomResourceDefinitionList, error)
}

// GatewayParams is the parameters for creating a Gateway.
type GatewayParams struct {
	fx.In

	Logger    *zap.Logger
	Scheme    *runtime.Scheme
	K8sConfig *rest.Config
}

type gateway struct {
	logger        *zap.Logger
	scheme        *runtime.Scheme
	k8sConfig     *rest.Config
	apiExtClient  apiextensionsclientset.Interface
	dynamicClient dynamic.Interface
}

func NewCRDGateway(p GatewayParams) Gateway {
	apiExtClient := apiextensionsclientset.NewForConfigOrDie(p.K8sConfig)
	dynamicClient := dynamic.NewForConfigOrDie(p.K8sConfig)

	return &gateway{
		logger:        p.Logger.With(zap.String("module", moduleName)),
		scheme:        p.Scheme,
		k8sConfig:     p.K8sConfig,
		apiExtClient:  apiExtClient,
		dynamicClient: dynamicClient,
	}
}

// ConditionalUpsert create or update CRD, also check for CRD compatibility before update
func (r *gateway) ConditionalUpsert(
	ctx context.Context,
	crd *apiextv1.CustomResourceDefinition,
	enableIncompatibleUpdate bool) error {
	r.logger.Info("Get CRD schema from k8s server.", zap.String("name", crd.Name))

	// get existing CRDs in the cluster
	crdOnServer, err := r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
	if err != nil {
		// directly update if CRD not found
		if k8sErrors.IsNotFound(err) {
			r.logger.Info("CRD does not exist, create CRD.", zap.String("name", crd.Name))
			_, err = r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Create(
				ctx,
				crd,
				metav1.CreateOptions{},
			)
			if err != nil {
				e := fmt.Errorf("failed to create CRD %s: %w", crd.Name, err)
				r.logger.Error(e.Error())
				return e
			}
			return nil
		}

		e := fmt.Errorf("failed to get CRD %s: %w", crd.Name, err)
		r.logger.Error(e.Error())
		return e
	}

	// return error if there are CRD versions that are in the cluster but is not in the corresponding new CRDs,
	// as we don't currently support removing versions of CRDs in the cluster
	for _, v := range crdOnServer.Spec.Versions {
		find := false
		for _, newV := range crd.Spec.Versions {
			if v.Name == newV.Name {
				find = true
				break
			}
		}
		if !find {
			e := fmt.Errorf("CRD %s has version %s that is not in the new CRD", crd.Name, v.Name)
			r.logger.Error(e.Error())
			return e
		}
	}

	// Compare change, then apply update conditionally
	r.logger.Info("CRD exists, compare CRD schema", zap.String("name", crd.Name))
	compareResult, err := CompareCRDSchemas(crdOnServer, crd)
	if err != nil {
		return err
	}

	if !compareResult.HasChange {
		r.logger.Info("Skip schema update. No change in CRD.", zap.String("name", crd.Name))
		return nil
	}

	if !compareResult.Compatible && !enableIncompatibleUpdate {
		has, e := r.hasInstances(ctx, crdOnServer)
		if e != nil {
			return e
		}
		if has {
			r.logSchemaDiff(crdOnServer, crd, compareResult.IncompatibilityDetails)
			return fmt.Errorf("failed to update CRD %s: schema incompatible with existing instances%s",
				crd.Name, formatIncompatibilityDetails(compareResult.IncompatibilityDetails))
		}
	}

	r.logger.Info("Update CRD definition.", zap.String("name", crd.Name))
	crd.ResourceVersion = crdOnServer.ResourceVersion // for k8s concurrency control
	updatedCRD, err := r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Update(
		ctx,
		crd,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return err
	}
	r.logger.Info("CRD updated", zap.String("name", updatedCRD.Name))
	return nil
}

// Delete CRDs on server, also check for instance of the CRD before deletion
func (r *gateway) Delete(ctx context.Context, crdToDelete *apiextv1.CustomResourceDefinition) error {
	hasInstances, err := r.hasInstances(ctx, crdToDelete)
	if err != nil {
		return err
	}

	if hasInstances {
		// there are resources, can not delete CRD
		return fmt.Errorf("failed to delete CRD %s. There are existing resources", crdToDelete.Name)
	}

	r.logger.Info("Delete CRD", zap.String("name", crdToDelete.Name))
	return r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, crdToDelete.Name, metav1.DeleteOptions{})
}

// List list all CRDs on server
func (r *gateway) List(ctx context.Context) (*apiextv1.CustomResourceDefinitionList, error) {
	listResponse, err := r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list existing CRDs: %w", err)
	}

	return listResponse, nil
}

func formatIncompatibilityDetails(details []VersionIncompatibility) string {
	if len(details) == 0 {
		return ""
	}
	var parts []string
	for _, d := range details {
		if len(d.Reasons) > 0 {
			parts = append(parts, fmt.Sprintf("version %s: [%s]", d.Version, strings.Join(d.Reasons, "; ")))
		} else {
			parts = append(parts, fmt.Sprintf("version %s: incompatible (no details)", d.Version))
		}
	}
	return "; " + strings.Join(parts, ", ")
}

// logSchemaDiff emits a debug log with the full old and new OpenAPI schemas for each
// incompatible version, enabling side-by-side comparison without a redeploy.
// Enable via the /debug/logging endpoint (uMonitor debug admin tab).
func (r *gateway) logSchemaDiff(oldCRD, newCRD *apiextv1.CustomResourceDefinition, details []VersionIncompatibility) {
	if !r.logger.Core().Enabled(zap.DebugLevel) {
		return
	}
	oldVersions := make(map[string]*apiextv1.CustomResourceDefinitionVersion, len(oldCRD.Spec.Versions))
	for i := range oldCRD.Spec.Versions {
		oldVersions[oldCRD.Spec.Versions[i].Name] = &oldCRD.Spec.Versions[i]
	}
	newVersions := make(map[string]*apiextv1.CustomResourceDefinitionVersion, len(newCRD.Spec.Versions))
	for i := range newCRD.Spec.Versions {
		newVersions[newCRD.Spec.Versions[i].Name] = &newCRD.Spec.Versions[i]
	}
	for _, d := range details {
		oldSchema, newSchema := "{}", "{}"
		if v, ok := oldVersions[d.Version]; ok && v.Schema != nil {
			if b, err := json.Marshal(v.Schema.OpenAPIV3Schema); err == nil {
				oldSchema = string(b)
			}
		}
		if v, ok := newVersions[d.Version]; ok && v.Schema != nil {
			if b, err := json.Marshal(v.Schema.OpenAPIV3Schema); err == nil {
				newSchema = string(b)
			}
		}
		r.logger.Debug("incompatible CRD schema diff",
			zap.String("name", oldCRD.Name),
			zap.String("version", d.Version),
			zap.Strings("reasons", d.Reasons),
			zap.String("old_schema", oldSchema),
			zap.String("new_schema", newSchema),
		)
	}
}

func (r *gateway) hasInstances(ctx context.Context, crd *apiextv1.CustomResourceDefinition) (bool, error) {
	for _, v := range crd.Spec.Versions {
		if !v.Storage {
			continue
		}
		gvr := schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  v.Name,
			Resource: crd.Spec.Names.Plural,
		}
		result, err := r.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			if utils.IsNotFoundError(err) {
				continue
			}
			return false, fmt.Errorf("failed to list existing instances of CRD %s: %w", crd.Name, err)
		}
		if len(result.Items) > 0 {
			return true, nil
		}
	}
	return false, nil
}
