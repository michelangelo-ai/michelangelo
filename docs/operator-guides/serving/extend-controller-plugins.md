---
last_verified: 2026-10-22
---

# Extend the Deployment and Inference Server Controllers

This guide explains the plugin interfaces inside the Deployment and InferenceServer controllers: what each one does, how the stock controller manager wires it, and how to replace or add to it.

:::tip Start with the Backend interface
Most serving integrations should implement `Backend`, `ModelConfigProvider`, or the routing `Manager` described in [Integrate with Your Custom Backend](./integrate-custom-backend.md), not replace the controller plugins on this page. The stock InferenceServer plugin already delegates to those interfaces. Use this page when you need a deployment target the stock build doesn't handle (for example, `TARGET_TYPE_OFFLINE`), or when you need to change how the controllers themselves behave.
:::

## How these plugins are wired

**None of these plugins is selected by configuration.** There is no Helm value, config key, or environment variable that picks a deployment plugin, an InferenceServer plugin, or an endpoint provider. The only `inferenceServer` config block ([`go/base/config/config.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/config/config.go)) sets gateway and Triton settings.

Instead, every implementation is an [fx](https://pkg.go.dev/go.uber.org/fx) value in the controller manager binary. To add or replace one, you build **your own controller manager**:

1. Copy [`go/cmd/controllermgr/main.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/cmd/controllermgr/main.go) into your own `main` package.
2. Edit the `fx.Options` list in `options()`. The stock serving wiring is:

   ```go
   deploymentOSSPlugin.Module,
   deployment.Module,
   backends.Module,
   modelconfig.Module,
   gatewayapi.Module,
   inferenceserverOSSPlugin.Module,
   inferenceserver.Module,
   ```

3. Build and deploy that binary in place of the stock `controllermgr` image.

The sections below say, for each interface, whether you **add** to the stock wiring (deployment plugins), **swap** a module (the InferenceServer plugin), or **decorate** a value (endpoint provider and publisher).

## Overview

| Interface | Purpose | Reference Implementation |
| --------- | ------- | ------------------------ |
| `ConditionActor` / `Plugin[T]` | Shared building block: an ordered list of steps that drive a resource toward a desired state | Every controller plugin below |
| Deployment `Plugin` | Roll out, roll back, clean up, and monitor a Deployment for one target type and subtype | OSS plugin (`TARGET_TYPE_INFERENCE_SERVER`) |
| `Registrar` | Maps `<targetType>:<subType>` to a Deployment `Plugin` | `SimpleRegistrar` |
| InferenceServer `Plugin` | Create and delete an InferenceServer's infrastructure | OSS plugin (delegates to `Backend`) |
| `Provider` / `Publisher` | Resolve each cluster's ingress endpoint and publish the per-server discovery Service | `K8sProvider` / default publisher |

---

## 1. Condition Actor Interface

Both controllers are built from condition actors. A condition plugin returns an ordered list of actors; each actor owns one `api.Condition` (identified by `GetType()`) in the resource's status.

**Interface:** [`go/base/conditions/interfaces/interfaces.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/conditions/interfaces/interfaces.go)

```go
type ConditionActor[T client.Object] interface {
	Retrieve(ctx context.Context, resource T, previousCondition *api.Condition) (*api.Condition, error)
	Run(ctx context.Context, resource T, previousCondition *api.Condition) (*api.Condition, error)
	GetType() string
}

type Plugin[T client.Object] interface {
	GetActors() []ConditionActor[T]
	GetConditions(resource T) []*api.Condition
	PutCondition(resource T, condition *api.Condition)
}
```

**Reference Implementation:** [`go/base/conditions/engine/defaultengine.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/conditions/engine/defaultengine.go) (the engine that drives them) and [`go/components/inferenceserver/plugins/oss/creation/condition_plugin.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/plugins/oss/creation/condition_plugin.go) (a plugin with seven actors)

### How the engine runs actors

On every reconcile, `DefaultEngine.Run` walks `GetActors()` in order:

1. For **every** actor it looks up the actor's previous condition by `GetType()` (or starts from `CONDITION_STATUS_UNKNOWN`) and calls `Retrieve`. `Retrieve` should be idempotent and side-effect free.
2. For the **first** actor whose retrieved condition isn't `CONDITION_STATUS_TRUE`, it calls `Run` with that condition. `Run` does the work. No later actor's `Run` is called in the same reconcile; their `Retrieve` still is.
3. It calls `PutCondition` with each actor's latest condition.

The engine then turns the outcome into a `Result`:

| Outcome | Result |
| ------- | ------ |
| Every `Retrieve` returned `TRUE` (no `Run` called) | `AreSatisfied`, `IsTerminal` |
| `Run` returned `FALSE` | `IsTerminal`, not satisfied (and `IsKilled` if the reason is the engine's `KillReason`) |
| `Run` returned `UNKNOWN` or `TRUE` | Requeue after 10 seconds; the next reconcile moves on |
| `Retrieve` or `Run` returned an error | `IsTerminal`, not satisfied, no requeue |

So `UNKNOWN` means "in progress, check again", and `FALSE` means "failed, stop". The interface comment says returned errors are used only for logging, but the engine treats **any** error as terminal. Return an error only when you want reconciliation to stop; report retryable failures as an `UNKNOWN` condition instead.

### Minimal actor

The helpers in [`go/base/conditions/utils/utils.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/conditions/utils/utils.go) update the condition in place and bump its timestamp when it changes. The `myActor` type and its checks are illustrative:

```go
type myActor struct{}

func (a *myActor) GetType() string { return "MyStep" }

func (a *myActor) Retrieve(ctx context.Context, d *v2pb.Deployment, prev *api.Condition) (*api.Condition, error) {
	done := false // ... check whether the step is already complete, without side effects
	if done {
		return conditionsutil.GenerateTrueCondition(prev), nil
	}
	return conditionsutil.GenerateUnknownCondition(prev, "step not complete", "Pending"), nil
}

func (a *myActor) Run(ctx context.Context, d *v2pb.Deployment, prev *api.Condition) (*api.Condition, error) {
	// ... do the work; on a permanent failure return GenerateFalseCondition
	return conditionsutil.GenerateUnknownCondition(prev, "step started", "InProgress"), nil
}
```

### Testing

[`go/base/conditions/engine/defaultengine_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/conditions/engine/defaultengine_test.go) shows how the engine treats each status. Actor tests such as [`endpoint_publish_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/plugins/oss/creation/endpoint_publish_test.go) call `Retrieve` and `Run` directly.

---

## 2. Deployment Plugin Interface

The Deployment `Plugin` is the controller's main extension point. The controller picks one plugin per Deployment, by target type and subtype, and asks it for a condition plugin for each lifecycle phase.

**Interface:** [`go/components/deployment/plugins/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/plugins/interface.go)

```go
type Plugin interface {
	GetState(ctx context.Context, observability ObservabilityContext, modelDeployment *v2pb.Deployment) (v2pb.DeploymentStatus, error)
	HealthCheckGate(ctx context.Context, observability ObservabilityContext, modelDeployment *v2pb.Deployment) (bool, error)
	GetRolloutPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error)
	GetRollbackPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error)
	GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment]
	GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment]
	ParseStage(resource *v2pb.Deployment) v2pb.DeploymentStage
	PopulateDeploymentLogs(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)
	PopulateMessage(ctx context.Context, runtimeContext RequestContext, modelDeployment *v2pb.Deployment)
}

type RequestContext struct {
	Deployment *v2pb.Deployment
	Logger     logr.Logger
}

type ObservabilityContext struct {
	Logger logr.Logger
	Scope  interface{}
}
```

**Reference Implementation:** [`go/components/deployment/plugins/oss/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/components/deployment/plugins/oss) (real rollout, rollback, cleanup, and steady-state actors) and [`go/components/deployment/plugins/noop/plugin.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/plugins/noop/plugin.go) (the smallest complete skeleton)

### How the controller calls it

On each reconcile ([`go/components/deployment/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/controller.go)):

- **Cleanup** (the Deployment is being deleted, is marked deleted in its spec, or has no desired revision but still has a current or candidate one) → runs `GetCleanupPlugin()`.
- **Rollout in progress** → calls `HealthCheckGate`. If it returns `false` and there is a previous revision to return to, or the desired revision changed mid-rollout, it runs `GetRollbackPlugin`; otherwise `GetRolloutPlugin`. (An emergency rollout without a rollback trigger never switches to rollback.) A rollback that has started runs to completion.
- **Rollout failed** → runs `GetRollbackPlugin` to restore the previous revision.
- **New desired revision** → sets the stage to validation and runs `GetRolloutPlugin`.
- **Steady state** → runs `GetSteadyStatePlugin()`.
- After the condition plugin runs, it calls `ParseStage` to compute the new stage. **Your plugin owns stage progression**: the controller only moves to `DEPLOYMENT_STAGE_ROLLOUT_COMPLETE`, `..._ROLLBACK_COMPLETE`, or `..._CLEAN_UP_COMPLETE` when `ParseStage` returns it.
- On a terminal failure it calls `PopulateDeploymentLogs`; on entering `ROLLBACK_COMPLETE` or `ROLLOUT_FAILED` it calls `PopulateMessage`.
- Finally it calls `GetState` and **replaces `deployment.Status` with the returned value**. Return the full status (start from `modelDeployment.Status`), not just the fields you changed.

Conditions whose type isn't one of the current condition plugin's actors are removed from the status after each run, so give every actor a distinct `GetType()`.

### Registration: `Registrar`

**Interface:** [`go/base/pluginmanager/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/base/pluginmanager/interface.go)

```go
type Registrar[T any] interface {
	RegisterPlugin(targetType, subType string, plugin T) error
	GetPlugin(targetType, subType string, deployment *v2pb.Deployment) (T, error)
}
```

`deployment.Module` provides one `pluginmanager.Registrar[plugins.Plugin]` (a `SimpleRegistrar`). The key is `"<targetType>:<subType>"`, where `targetType` is the enum's string name, for example `v2pb.TARGET_TYPE_OFFLINE.String()`.

**Selection is per Deployment** (`getPlugin` in `controller.go`):

- If `spec.definition` is unset, the controller looks up `TARGET_TYPE_INFERENCE_SERVER` with subtype `""`.
- Otherwise it looks up `spec.definition.type` and `spec.definition.sub_type`.
- If nothing is registered under that key, reconciliation fails with `no plugin found for targetType=<type>, subType=<subtype>`.

**Stock registrations** ([`go/components/deployment/plugins/oss/register.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/plugins/oss/register.go)): the OSS plugin is registered for `TARGET_TYPE_INFERENCE_SERVER` with subtypes `realtime-serving`, `batch-serving`, and `""`. Nothing else is registered.

:::warning Offline and mobile deployments fail on a stock build
`noop.RegisterNoOpPlugins` would register `TARGET_TYPE_OFFLINE` and `TARGET_TYPE_MOBILE`, but no binary calls it. On a stock controller manager, a Deployment with `spec.definition.type` set to `TARGET_TYPE_OFFLINE`, `TARGET_TYPE_MOBILE`, or `TARGET_TYPE_SELF_HOSTED` fails with `no plugin found`. That is the gap a custom Deployment plugin fills.
:::

### To add a deployment plugin:

1. Implement all nine `Plugin` methods. Start from the `noop` package and replace the condition plugins with your own actors.
2. Write an fx module that takes the `Registrar` and registers your plugin in an `fx.Invoke`, mirroring `oss/register.go`. `NewMyPlugin` is your constructor:

   ```go
   var Module = fx.Options(
   	fx.Invoke(register),
   )

   func register(registrar pluginmanager.Registrar[plugins.Plugin]) error {
   	return registrar.RegisterPlugin(v2pb.TARGET_TYPE_OFFLINE.String(), "", NewMyPlugin())
   }
   ```

3. Add the module to the `fx.Options` in your copy of `main.go`, alongside `deploymentOSSPlugin.Module` and `deployment.Module`.

:::caution Duplicate keys silently replace
`SimpleRegistrar.RegisterPlugin` doesn't check for an existing key: a second registration for the same `<targetType>:<subType>` replaces the first and returns `nil`. fx runs `fx.Invoke` functions in the order they appear in `fx.Options`, so the **last** registration wins. To override the OSS plugin for an inference-server subtype, put your module **after** `deploymentOSSPlugin.Module`. To avoid overriding it by accident, register under your own subtype.
:::

### Testing

[`go/components/deployment/controller_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/controller_test.go) builds a `SimpleRegistrar` with `pluginmanager.NewSimpleRegistrar`, registers `noop.NewNoOpPlugin()`, and reconciles against a fake client; swap in your plugin to test it the same way. [`go/components/deployment/plugins/oss/plugin_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/deployment/plugins/oss/plugin_test.go) tests the OSS plugin's methods directly. There are no generated mocks for the Deployment `Plugin` interface.

---

## 3. InferenceServer Plugin Interface

The InferenceServer controller takes **exactly one** `plugins.Plugin`. Unlike the Deployment controller, there is no registrar: you replace the plugin for the whole binary.

**Interface:** [`go/components/inferenceserver/plugins/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/plugins/interface.go)

```go
type Plugin interface {
	GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer]
	GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer]
	ParseState(resource *v2pb.InferenceServer) v2pb.InferenceServerState
	UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error
	UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer])
}
```

**Reference Implementation:** [`go/components/inferenceserver/plugins/oss/plugin.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/plugins/oss/plugin.go)

### How the controller calls it

On each reconcile ([`go/components/inferenceserver/controller.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/controller.go)) the controller calls `UpdateDetails` (errors are logged, not fatal), picks `GetDeletionPlugin` if the InferenceServer is being deleted or decommissioned and `GetCreationPlugin` otherwise, calls `UpdateConditions` with that plugin, runs the condition engine, and sets `status.state` from `ParseState`.

### Replacing it

`inferenceserverOSSPlugin.Module` is just `fx.Provide(NewOSSPlugin)`. To use your own plugin, remove `inferenceserverOSSPlugin.Module` from your copy of `main.go` and provide a constructor that returns `plugins.Plugin`:

```go
// inferenceserverOSSPlugin.Module, // removed
fx.Provide(NewMyInferenceServerPlugin), // returns plugins.Plugin
inferenceserver.Module,
```

If both are present, fx fails at startup because `plugins.Plugin` is provided twice.

### What you give up

The OSS plugin is the integration point for everything in [Integrate with Your Custom Backend](./integrate-custom-backend.md). Its creation plugin runs these actors in order (`plugins/oss/creation/`):

| Condition type | What it does |
| -------------- | ------------ |
| `Validation` | Validates the InferenceServer configuration; failures aren't retried |
| `BackendProvision` | Creates the server's Kubernetes resources through the selected `Backend` |
| `EndpointPublish` | Resolves each cluster with the endpoint `Provider` and syncs the `Publisher` |
| `DiscoveryRouteProvision` | Maintains the control-plane discovery HTTPRoute through the routing `Manager` |
| `TrafficRouteProvision` | Maintains the traffic HTTPRoute in each target cluster through the routing `Manager` |
| `ModelConfigProvision` | Provisions the model config through the `ModelConfigProvider` |
| `HealthCheck` | Checks server health through the `Backend`; failures aren't auto-remediated |

Its deletion plugin runs `RouteCleanup` and then `Cleanup` (`plugins/oss/deletion/`).

A replacement plugin has to reproduce whichever of these you still need. Treat it as an escape hatch: if a custom `Backend`, `ModelConfigProvider`, routing `Manager`, or endpoint provider does the job, use that instead.

### Testing

[`go/components/inferenceserver/controller_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/controller_test.go) drives the reconciler with hand-written mock plugins and actors. The OSS actors have tests next to them in `plugins/oss/creation/` and `plugins/oss/deletion/`.

---

## 4. Endpoint Provider and Publisher Interfaces

The `EndpointPublish` actor uses two interfaces to tell other components where each cluster admits traffic for an InferenceServer.

**Interface:** [`go/components/inferenceserver/endpoints/interface.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/endpoints/interface.go)

```go
type Endpoint struct {
	Host   string
	Port   int32
	Scheme string // "http" | "https"
}

type Provider interface {
	Resolve(ctx context.Context, target *v2pb.ClusterTarget) (Endpoint, error)
}

type Publisher interface {
	Sync(ctx context.Context, server *v2pb.InferenceServer, endpoints map[string]Endpoint) error
	Get(ctx context.Context, server *v2pb.InferenceServer) (map[string]Endpoint, error)
	Delete(ctx context.Context, server *v2pb.InferenceServer) error
}
```

**Reference Implementation:** [`go/components/inferenceserver/endpoints/provider/k8s.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/endpoints/provider/k8s.go) (`NewK8sProvider`) and [`go/components/inferenceserver/endpoints/publisher.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/endpoints/publisher.go) (`NewDefaultPublisher`)

### Contract

From the interface's godoc:

- Each InferenceServer is exposed at one discovery endpoint backed by a Service named `{inferenceServerName}-endpoints` in the InferenceServer's namespace. Downstream consumers reach the server through that Service, so a `Publisher` must preserve both the name and the namespace.
- `Sync` is idempotent: cluster IDs in the map are upserted, and cluster IDs previously published but absent from the map are removed.
- `Get` returns the published map, empty when nothing has been published yet.
- After `Delete` returns, the discovery Service no longer exists.

How the OSS actor uses them:

- `Run` calls `Resolve` for every `spec.cluster_targets` entry, then calls `Sync` once with a map keyed by cluster ID. A `Resolve` error for one cluster leaves the condition `UNKNOWN` (retried) rather than failing the server; a `Sync` error sets it `FALSE`.
- `Retrieve` calls `Get` and compares the published cluster IDs with the spec.
- The OSS deletion actors don't call `Publisher.Delete`. The default publisher sets the InferenceServer as the controller owner of the Service and EndpointSlices it creates, so Kubernetes garbage collection removes them. A custom `Publisher` that stores state elsewhere needs its own cleanup.

### Replacing them with `fx.Decorate`

Both defaults are provided **inside `inferenceserver.Module`**, which includes `endpoints.Module` (provides `endpoints.Publisher`) and `provider.Module` (provides `endpoints.Provider`). You can't leave them out without dropping the controller, so replace them with [`fx.Decorate`](https://pkg.go.dev/go.uber.org/fx#Decorate) in your copy of `main.go`. Both modules provide the **interface** types, so decorate `endpoints.Provider` and `endpoints.Publisher`, not the concrete `*provider.K8sProvider`.

A decorator receives the existing value and returns the replacement. It can ignore the default and ask for other dependencies. `NewMyProvider` is your constructor:

```go
inferenceserverOSSPlugin.Module,
inferenceserver.Module,
fx.Decorate(func(_ endpoints.Provider, logger *zap.Logger) endpoints.Provider {
	return NewMyProvider(logger)
}),
```

Put `fx.Decorate` at the top level of `fx.Options`, as above. The stock modules are plain `fx.Options` (not named `fx.Module`s), so a top-level decorator applies to every consumer, including the OSS InferenceServer plugin. The default is still constructed, because the decorator takes it as an input.

### Testing

Generated mocks for both interfaces are in [`go/components/inferenceserver/endpoints/endpointsmocks/`](https://github.com/michelangelo-ai/michelangelo/tree/main/go/components/inferenceserver/endpoints/endpointsmocks) (`//go:generate mamockgen Publisher Provider`). See [`publisher_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/endpoints/publisher_test.go) and [`provider/k8s_test.go`](https://github.com/michelangelo-ai/michelangelo/blob/main/go/components/inferenceserver/endpoints/provider/k8s_test.go) for the defaults' tests.

---

## Best Practices

1. **Prefer the narrowest extension point**: a `Backend`, `ModelConfigProvider`, routing `Manager`, or endpoint `Provider` before a whole controller plugin.

2. **Keep `Retrieve` side-effect free**: the engine calls it for every actor on every reconcile.

3. **Use `UNKNOWN` for retryable states**: `FALSE` and returned errors both end reconciliation as terminal.

4. **Register under your own subtype**: `RegisterPlugin` replaces silently, so a new subtype avoids clobbering the OSS plugin.

5. **Return the full status from `GetState`**: the Deployment controller overwrites `status` with it.

6. **Pin your copy of `main.go` to a release**: your binary carries the stock wiring, so diff it against `go/cmd/controllermgr/main.go` when you upgrade.
