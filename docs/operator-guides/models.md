---
sidebar_position: 5
---

# Model Management

Michelangelo AI provides three gRPC services for managing ML models, model families, and cached inference outputs. These services are used by training pipelines, the Python SDK, and the serving control plane.

## ModelService

`ModelService` manages Model resources — registered ML artifacts that can be deployed to an InferenceServer or referenced by pipeline tasks.

**Proto:** `proto/api/v2/model_svc.proto`

**Operations:** Create, Get, List, Update, Delete, Delete Collection

A Model records metadata about a trained artifact: its name, version, storage location, and framework. Models are typically created by training pipelines via the Python SDK (`michelangelo.lib.trainer`) and stored in the model registry. Once registered, a Model can be referenced by a [Deployment](./serving/index.md) to serve it on an InferenceServer.

```
ModelService
  CreateModel              – register a new model artifact
  GetModel                 – fetch model metadata by name/version
  ListModels               – list models, optionally filtered by family or label
  UpdateModel              – update labels, description, or storage metadata
  DeleteModel              – remove a model registration
  DeleteModelCollection    – bulk-delete multiple model registrations
```

## ModelFamilyService

`ModelFamilyService` manages ModelFamily resources — named groupings that logically relate a set of model versions.

**Proto:** `proto/api/v2/model_family_svc.proto`

**Operations:** Create, Get, List, Update, Delete, Delete Collection

A ModelFamily provides a stable namespace for iterating on a model: new trained versions are added to the same family, and serving configuration can target the family rather than a specific version. This decouples deployment lifecycle from training cadence.

```
ModelFamilyService
  CreateModelFamily              – create a new model family
  GetModelFamily                 – fetch family metadata
  ListModelFamilies              – list all families in a project
  UpdateModelFamily              – update description or labels
  DeleteModelFamily              – remove a family (does not delete member models)
  DeleteModelFamilyCollection    – bulk-delete multiple model families
```

## CachedOutputService

`CachedOutputService` manages CachedOutput resources — pre-computed inference results stored for reuse.

**Proto:** `proto/api/v2/cached_output_svc.proto`

**Operations:** Create, Get, List, Update, Delete, Delete Collection

A CachedOutput stores the result of a previous inference call, keyed by its input. The serving layer can check for a cached result before forwarding a request to the InferenceServer, reducing latency and compute cost for repeated inputs. The service supports full CRUD, allowing cached entries to be updated, deleted individually, or bulk-deleted.

```
CachedOutputService
  CreateCachedOutput              – store a new cached inference result
  GetCachedOutput                 – retrieve a cached result by key
  UpdateCachedOutput              – update a cached result
  DeleteCachedOutput              – remove a single cached result
  DeleteCachedOutputCollection    – bulk-delete multiple cached results
  ListCachedOutput                – list cached results, optionally filtered
```

## Related documentation

- [Serving Overview](./serving/index.md) — deploying models to InferenceServers
- [Model Registry Guide](../user-guides/train-and-deploy-models/model-registry-guide.md) — registering and versioning models via the Python SDK
- `proto/api/v2/` — full proto definitions for all services
