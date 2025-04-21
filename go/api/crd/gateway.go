//go:generate mamockgen Gateway
package crd

import (
	"context"
	"fmt"
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
func (r *gateway) ConditionalUpsert(ctx context.Context, crd *apiextv1.CustomResourceDefinition, enableIncompatibleUpdate bool) error {
	r.logger.Info("Get CRD schema from k8s server.", zap.String("name", crd.Name))
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

	// Compare change, then apply update conditionally
	r.logger.Info("CRD exists, compare CRD schema", zap.String("name", crd.Name))
	compareResult, err := compareCRDSchemas(crdOnServer, crd)
	if err != nil {
		return err
	}

	if !compareResult.hasChange {
		r.logger.Info("Skip schema update. No change in CRD.", zap.String("name", crd.Name))
		return nil
	}

	if !compareResult.compatible && !enableIncompatibleUpdate {
		has, e := r.hasInstances(ctx, crdOnServer)
		if e != nil {
			return e
		}
		if has {
			return fmt.Errorf("failed to update CRD. Schema is incompatible, and there are existing instances. Abort updating CRD %s", crd.Name)
		}
	}

	// k8s use ResourceVersion for concurrency control
	r.logger.Info("Update CRD definition.", zap.String("name", crd.Name))
	crd.ResourceVersion = crdOnServer.ResourceVersion
	updatedCRD, err := r.apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Update(
		ctx,
		crd,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return err
	}
	r.logger.Info("CRD updated successfully to version", zap.String("name", updatedCRD.Name), zap.String("version", updatedCRD.ResourceVersion))
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

func (r *gateway) hasInstances(ctx context.Context, crd *apiextv1.CustomResourceDefinition) (bool, error) {
	for _, v := range crd.Spec.Versions {
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
