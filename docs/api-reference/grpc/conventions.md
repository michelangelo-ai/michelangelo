---
sidebar_position: 0
sidebar_label: Overview
---

# How to Read the gRPC API Reference

The **gRPC API Reference** pages — [Model & Evaluation Services](./models.md), [Pipeline Services](./pipelines.md), [Serving Services](./serving.md), and [Jobs Services](./jobs.md) — document all 15 of Michelangelo AI's resource services, one RPC at a time. They share the conventions below, so this page explains them once instead of repeating the explanation on every page.

## The CRUD-plus-list pattern

Every resource service exposes the same six RPCs:

| RPC | Purpose |
|-----|---------|
| `Create<Resource>` | Create a new resource with the given spec |
| `Get<Resource>` | Fetch a single resource by name and namespace |
| `Update<Resource>` | Replace a resource's spec |
| `Delete<Resource>` | Delete a single resource |
| `Delete<Resource>Collection` | Delete every resource matching a list filter |
| `List<Resource>` | List resources within one namespace |

**Watch is not implemented, and `List` cannot span all namespaces** — every `List` call must be scoped to a single namespace. This holds for all 15 services covered by these pages.

## Reading the "Required" column

Proto3 doesn't mark fields as required at the wire level. The **Required** column in every table reflects logical necessity — inferred from field semantics and proto comments — not a protocol-level constraint.

## Fields with no proto comment

Every RPC also accepts one or more control fields inherited from the Kubernetes API machinery — `CreateOptions`, `GetOptions`, `UpdateOptions`, `DeleteOptions`, `ListOptions` — or from Michelangelo AI's own `ListOptionsExt`, plus a `<Type>List` wrapper on every `List` response. None of these carry a proto comment upstream, so their Description column reads `—` rather than repeating a placeholder on every occurrence.
