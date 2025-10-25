package crd

import (
	"context"
	"fmt"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"

	"github.com/cenkalti/backoff"
	"go.uber.org/config"
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
	EnableCRDUpdate   bool                     `yaml:"enableCRDUpdate"`
	EnableCRDDeletion bool                     `yaml:"enableCRDDeletion"`
	CRDVersions       map[string]VersionConfig `yaml:"crdVersions"`
}

// VersionConfig defines how the versions of a CRD will be handled.
type VersionConfig struct {
	// Versions is a list of versions of the CRD that will be synced to the cluster.
	Versions []string `yaml:"versions"`
	// StorageVersion is the version that will be used as the storage version for the CRD.
	StorageVersion string `yaml:"storageVersion"`
}

func ParseConfig(provider config.Provider) (*Configuration, error) {
	conf := Configuration{}
	err := provider.Get(crdSyncConfigKey).Populate(&conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

type SyncCRDsParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Config    *Configuration
	Logger    *zap.Logger
	Gateway   Gateway
}

// SyncCRDs syncs CRDs in the specified k8s API group
//
// This function is used to ensure that the CRDs in the cluster match the CRDs defined in the yamlSchemas.
// yamlSchemas is a list of CRD kind (name) to CRD yaml schema map. Each map corresponds to a version of the CRDs.
//
// In the specified k8s API group:
//  1. If Config.enableCRDDeletion is true, for each CRD that is in the k8s cluster but is not in yamlSchemas,
//     if the CRD does not have any instances in the cluster, the CRD will be deleted, otherwise returns an error.
//  2. For each CRD that is in yamlSchemas but is not in the cluster, it will be created in the cluster.
//  3. If multiple versions of the same CRD are defined in yamlSchemas, they will be merged into a single CRD object,
//     according to the rules defined in Config.CRDVersions.
//  4. CRDs that do not have a corresponding entry in Config.CRDVersions will be allowed to have only one version.
//  5. If a CRD has multiple versions in Config.CRDVersions, one of them must be specified as the storage version.
//  6. For each CRD that is in both yamlSchemas and the cluster, the schema of each matching version will be compared:
//     If the schema change is backward compatible, the CRD version will be updated
//     If the schema change is not backward compatible
//     If enableIncompatibleUpdate is true, the CRD version will be updated
//     If there is no existing instance in the cluster, the CRD will be updated
//     Otherwise, it will return an error
//  7. If a CRD version in the cluster does not have a corresponding entry in yamlSchemas, an error will be returned,
//     as we don't currently support removing versions of CRDs in the cluster.
func SyncCRDs(group string, incompatibleUpdateAllowList []string, yamlSchemas ...map[string]string) fx.Option {
	return fx.Invoke(func(p SyncCRDsParams) error {
		logger := p.Logger.With(zap.String("module", moduleName))
		logger.Info("CRD sync config", zap.Any("config", p.Config))
		if p.Config.EnableCRDUpdate {
			p.Lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					return syncCRDs(ctx, logger, group, p.Config.EnableCRDDeletion,
						p.Gateway, p.Config.CRDVersions, incompatibleUpdateAllowList, yamlSchemas...)
				},
				OnStop: nil,
			})
		}

		return nil
	})
}

func syncCRDs(ctx context.Context,
	logger *zap.Logger,
	group string,
	enableCRDDeletion bool,
	gateway Gateway,
	crdVersions map[string]VersionConfig,
	incompatibleUpdateAllowList []string,
	yamlSchemas ...map[string]string) error {

	// decode CRD yaml schemas into CRD objects
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
			if group != crd.Spec.Group {
				e := fmt.Errorf("CRD %s is not in the specified group [%v]", crd.Name, group)
				logger.Error(e.Error())
				return e
			}
			crdList = append(crdList, &crd)
		}
	}

	// get existing CRDs in the cluster
	existingCRDs, err := gateway.List(ctx)
	if err != nil {
		logger.Error("Failed to list existing CRDs", zap.Error(err))
		return err
	}

	// delete CRDs that are in the cluster but not in crdList
	for _, crd := range existingCRDs.Items {
		if group != crd.Spec.Group {
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
			if enableCRDDeletion {
				logger.Info("CRD deletion enabled. Delete CRD", zap.String("name", crd.Name))
				if err = gateway.Delete(ctx, &crd); err != nil && !utils.IsNotFoundError(err) {
					logger.Error("Fail to delete CRD", zap.String("name", crd.Name), zap.Error(err))
					return err
				}
			} else {
				logger.Info("CRD deletion disabled. Skip deleting CRD", zap.String("name", crd.Name))
			}
		}
	}

	mergedCRDList, err := mergeCRDVersions(crdList, crdVersions)
	if err != nil {
		logger.Error("Failed to merge CRD versions", zap.Error(err))
		return err
	}

	// upsert CRDs
	if err = upsertCRDs(ctx, logger, gateway, mergedCRDList, incompatibleUpdateAllowList); err != nil {
		logger.Error("Failed to upsert CRDs", zap.Error(err))
		return err
	}
	return nil
}

// upsertCRDs upsert CRD onto k8s cluster, also handle OCC conflict with retry
func upsertCRDs(ctx context.Context, logger *zap.Logger, gateway Gateway,
	crds []*apiextv1.CustomResourceDefinition, allowList []string) error {
	logger.Info("Compare CRD in cluster with new CRD definition, and conditionally update CRDs")
	for _, crd := range crds {
		// Per-CRD decision: only allowlist matters
		enableIncompatibleUpdate := isInAllowList(crd.Name, allowList)

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

// mergeCRDVersions merges the versions of CRDs in crdList according to the rules specified in crdVersions.
//
// crdVersions is a map where the key is the CRD name and the value is a VersionConfig. If a CRD in crdList doesn't
// have a corresponding entry in crdVersions, it will only be allowed to have a single version, and that version will
// be set as the storage version. If a CRD in crdList has a corresponding entry in crdVersions, all the versions
// specified in crdVersions[crd.Name].Versions must exist in crdList. If a CRD has multiple versions in
// crdVersions[crd.Name].Versions, one of them must be specified as the storage version. If a CRD has only one version
// in crdVersions[crd.Name], that version is automatically set as the storage version.
//
// The function returns a list of merged CRDs, where all the versions of each CRD are combined into a single
// CustomResourceDefinition object. If any of the rules are violated, an error is returned.
func mergeCRDVersions(
	crdList []*apiextv1.CustomResourceDefinition,
	crdVersions map[string]VersionConfig) ([]*apiextv1.CustomResourceDefinition, error) {
	var result []*apiextv1.CustomResourceDefinition
	// group CRDs by names and versions
	crds := make(map[string]map[string]*apiextv1.CustomResourceDefinition)
	for _, crd := range crdList {
		if _, exists := crds[crd.Name]; !exists {
			crds[crd.Name] = make(map[string]*apiextv1.CustomResourceDefinition)
		}
		if len(crd.Spec.Versions) != 1 {
			return nil, fmt.Errorf(
				"each CRD item must only have one version. CRD %s has %d versions",
				crd.Name, len(crd.Spec.Versions))
		}
		if _, exists := crds[crd.Name][crd.Spec.Versions[0].Name]; exists {
			return nil, fmt.Errorf("CRD %s has duplicated definitions of version %s",
				crd.Name, crd.Spec.Versions[0].Name)
		}
		crds[crd.Name][crd.Spec.Versions[0].Name] = crd
	}

	// merge versions for each CRD
	for name, versions := range crds {
		var mergedCRD *apiextv1.CustomResourceDefinition
		// check if the CRD has a corresponding entry in crdVersions
		if versionConfig, exists := crdVersions[name]; exists {
			// check if the versions in crdVersions match the versions in crds
			for _, version := range versionConfig.Versions {
				if _, verExists := versions[version]; !verExists {
					return nil, fmt.Errorf(
						"version %s of CRD %s is specified in crdVersions, but does not exist not in crdList",
						version, name)
				}
			}

			mergedCRD = versions[versionConfig.Versions[0]].DeepCopy()
			for _, version := range versionConfig.Versions[1:] {
				mergedCRD.Spec.Versions = append(mergedCRD.Spec.Versions, versions[version].Spec.Versions[0])
			}

			storageVersion := versionConfig.StorageVersion
			if storageVersion == "" {
				if len(mergedCRD.Spec.Versions) == 1 {
					mergedCRD.Spec.Versions[0].Storage = true // set storage version
				} else {
					return nil, fmt.Errorf(
						"CRD %s has multiple versions, but no storageVersion specified in crdVersions",
						name)
				}
			} else {
				foundStorage := false
				for i, v := range mergedCRD.Spec.Versions {
					if v.Name == storageVersion {
						mergedCRD.Spec.Versions[i].Storage = true
						foundStorage = true
						break
					}
				}
				if !foundStorage {
					return nil, fmt.Errorf("CRD %s does not have the specified storage version %s",
						name, storageVersion)
				}
			}
			// Only set conversion strategy when there are multiple versions
			if len(mergedCRD.Spec.Versions) > 1 {
				// TODO: conversion strategy is set to NoneConverter for now,
				// we will support webhook conversion in the next iteration
				mergedCRD.Spec.Conversion = &apiextv1.CustomResourceConversion{
					Strategy: apiextv1.NoneConverter,
				}
			}
		} else {
			// if the CRD does not have a corresponding entry in crdVersions, it must have only one version
			// and that version will be set as the storage version
			if len(versions) > 1 {
				return nil, fmt.Errorf(
					"CRD %s has multiple versions but version config specified in crdVersions",
					name)
			}

			for _, crd := range versions {
				mergedCRD = crd.DeepCopy()
				mergedCRD.Spec.Versions[0].Storage = true // set the only version to be the storage version
				break
			}
		}
		result = append(result, mergedCRD)
	}
	return result, nil
}

// isInAllowList checks if the given CRD name is in the incompatible update allowlist
func isInAllowList(crdName string, allowList []string) bool {
	for _, allowed := range allowList {
		if crdName == allowed {
			return true
		}
	}
	return false
}
