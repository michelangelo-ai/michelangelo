package crd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"go.uber.org/config"

	"github.com/cenkalti/backoff"
	"go.uber.org/fx"
	"go.uber.org/zap"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	crdSyncConfigKey = "apiserver.crdSync"
	k8sMaxRetries    = 3
)

// Configuration crd register configuration
type Configuration struct {
	EnableCRDUpdate          bool `yaml:"enableCRDUpdate"`
	EnableIncompatibleUpdate bool `yaml:"enableIncompatibleUpdate"`
}

func ParseConfig(provider config.Provider) (*Configuration, error) {
	conf := Configuration{}
	err := provider.Get(crdSyncConfigKey).Populate(&conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

// upsertCRDs upsert CRD onto k8s cluster, also handle OCC conflict with retry
func upsertCRDs(ctx context.Context, logger *zap.Logger, gateway Gateway,
	crds []*apiextv1.CustomResourceDefinition, enableIncompatibleUpdate bool) error {
	logger.Info("Compare CRD in cluster with new CRD definition, and conditionally update CRDs")
	for _, crd := range crds {
		err := backoff.Retry(func() error {
			err := gateway.ConditionalUpsert(ctx, crd, enableIncompatibleUpdate)
			if err != nil {
				if k8sErrors.IsConflict(err) || k8sErrors.IsServerTimeout(err) ||
					k8sErrors.IsTooManyRequests(err) || k8sErrors.IsUnexpectedServerError(err) ||
					k8sErrors.IsInternalError(err) || k8sErrors.IsServiceUnavailable(err) {
					return err
				}

				return backoff.Permanent(err)
			}

			return nil
		}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), k8sMaxRetries))
		if err != nil {
			logger.Error("Fail to update CRD definition.", zap.String("name", crd.Name), zap.Error(err))
			return err
		}
	}

	return nil
}

type SyncCRDsParams struct {
	fx.In

	Config  *Configuration
	Logger  *zap.Logger
	Gateway Gateway
}

// SyncCRDs sync CRDs to k8s cluster
// yamlSchemas is a list of CRD yaml schemas
//
//	In the specified k8s API groups:
//	1. For each CRD that is in the cluster but is not in yamlSchemas,
//	   If the CRD does not have existing instances in the cluster, the CRD will be deleted,
//	   otherwise returns an error
//	2. For each CRD that is in yamlSchemas but is not in the cluster, it will be created in the cluster
//	3. For each CRD that is in both yamlSchemas and the cluster, the schema will be compared
//	   If the schema change is backward compatible, the CRD will be updated
//	   If the schema change is not backward compatible
//	   - If enableIncompatibleUpdate is true, the CRD will be updated
//	   - If there is no existing instance in the cluster, the CRD will be updated
//	   - Otherwise, it will return an error
func SyncCRDs(groups []string, yamlSchemas ...map[string]string) fx.Option {
	return fx.Invoke(func(p SyncCRDsParams) error {
		if p.Config.EnableCRDUpdate {
			logger := p.Logger.With(zap.String("module", moduleName))

			return syncCRDs(logger, groups, p.Config.EnableIncompatibleUpdate, p.Gateway, yamlSchemas...)
		}

		return nil
	})
}

func syncCRDs(logger *zap.Logger, groups []string, enableIncompatibleUpdate bool, gateway Gateway,
	yamlSchemas ...map[string]string) error {
	ctx := context.Background()

	var crdList []*apiextv1.CustomResourceDefinition
	for _, schemaMap := range yamlSchemas {
		for kind, yamlStr := range schemaMap {
			crd := apiextv1.CustomResourceDefinition{}
			err := yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlStr)).Decode(&crd)
			if err != nil {
				logger.Error("Fail to deserialize CRD from yaml",
					zap.String("kind", kind), zap.String("yaml", yamlStr), zap.Error(err))
				return err
			}
			if !slices.Contains(groups, crd.Spec.Group) {
				e := fmt.Errorf("CRD %s is not in the specified groups [%v]", crd.Name, groups)
				logger.Error(e.Error())
				return e
			}
			crdList = append(crdList, &crd)
		}
	}

	existingCRDs, err := gateway.List(ctx)
	if err != nil {
		logger.Error("Failed to list existing CRDs", zap.Error(err))
		return err
	}

	for _, crd := range existingCRDs.Items {
		if !slices.Contains(groups, crd.Spec.Group) {
			continue
		}
		found := false
		for _, newCRD := range crdList {
			if crd.Name == newCRD.Name {
				found = true
				break
			}
		}
		if !found {
			if err = gateway.Delete(ctx, &crd); err != nil {
				logger.Error("Fail to delete CRD", zap.String("name", crd.Name), zap.Error(err))
				return err
			}
		}
	}

	if err = upsertCRDs(ctx, logger, gateway, crdList, enableIncompatibleUpdate); err != nil {
		logger.Error("Failed to upsert CRDs", zap.Error(err))
		return err
	}
	return nil
}
