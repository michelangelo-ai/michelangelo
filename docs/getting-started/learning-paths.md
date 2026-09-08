---
sidebar_position: 6
title: Learning Paths
description: Ordered tracks through the existing docs — pick the one that matches your role and follow it end to end.
---

# Learning Paths

The docs cover a lot of ground. These three tracks sequence the existing pages into an ordered path for the three most common roles, so you always know what to read next.

Pick the track that matches what you want to do:

| Track | You want to... | Time | Prerequisites |
|-------|----------------|------|---------------|
| [ML Engineer](#ml-engineer-track) | Build, train, and deploy models on the platform | ~2-3 hours | Python; Docker for the sandbox steps |
| [Operator](#operator-track) | Deploy and run the platform for your organization | ~3-4 hours | Kubernetes and Helm familiarity |
| [Contributor](#contributor-track) | Change the platform itself — code, plugins, docs | ~2 hours reading + environment setup | Go or Python, depending on the area |

## ML Engineer Track

From zero to a trained, deployed model.

1. **[Overview](./overview.md)** — what Michelangelo AI is and how tools you already know map to it (~10 min)
2. **[Core Concepts and Key Terms](./core-concepts-and-key-terms.md)** — projects, workflows, tasks, pipelines (~10 min)
3. **[Sandbox Setup](./sandbox-setup.md)** — a local cluster with all services running (~20 min)
4. **[Getting Started with Pipelines](../user-guides/getting-started/getting-started.md)** — build and run your first pipeline (~30 min)
5. **[Prepare Your Data](../user-guides/getting-started/prepare-your-data.md)** — load, clean, and split datasets with Ray and Spark
6. **[Train and Register a Model](../user-guides/train-and-deploy-models/train-and-register-a-model.md)** — train at scale and register artifacts
7. **[Distributed Training with LightningTrainer](../user-guides/train-and-deploy-models/distributed-training.md)** — optional deep dive: strategies, warm starts, and auto-resume for larger runs
8. **[Deploy a Model](../user-guides/train-and-deploy-models/deploy-a-model.md)** — bind a registered model to an inference server
9. **[Example Projects](../user-guides/examples/index.md)** — ten end-to-end workflows to adapt to your own use case

**You're done when:** you've run a pipeline that trains a model, registered it, deployed it, and know which example is closest to your real workload.

## Operator Track

From an empty Kubernetes cluster to a monitored production deployment. The first four steps follow the [operator guides' recommended reading order](../operator-guides/index.md).

1. **[Platform Setup](../operator-guides/setup/platform-setup.md)** — configure each component via ConfigMaps and Kustomize overlays
2. **[Register a Compute Cluster](../operator-guides/setup/register-a-compute-cluster-to-michelangelo-control-plane.md)** — connect a cluster for Ray and Spark jobs
3. **[Cluster Setup for Serving](../operator-guides/serving/cluster-setup.md)** — enable model inference
4. **[Authentication](../operator-guides/setup/authentication.md)** — identity provider and RBAC before opening to users
5. **[Network & Ingress](../operator-guides/setup/network.md)** — Envoy, Ingress, TLS
6. **[Helm Chart](../operator-guides/helm-chart.md)** — chart layout, values reference, migration phases
7. **[Monitoring](../operator-guides/operations/monitoring.md)** — metrics, alerts, and dashboards for the control plane
8. **[Troubleshooting](../operator-guides/operations/troubleshooting.md)** — the failure modes you'll actually hit

**You're done when:** the control plane is deployed, a compute cluster is registered, users authenticate through your IdP, and you have monitoring in place.

## Contributor Track

From reader to merged PR.

1. **[Contributing Overview](../contributing/index.md)** — what you can contribute and which directory owns what
2. **[Terminology](../contributing/TERMINOLOGY.md)** — the vocabulary the codebase uses, before you read code
3. **[Building from Source](../contributing/building-michelangelo-ai-from-source.md)** — verify your environment works
4. **[Dev Environment](../contributing/dev-environment.md)** — IDE setup for Go backend development
5. **[Testing Strategy](../contributing/testing.md)** — what to test and where the suites live
6. **[PR Process](../contributing/pr-process.md)** — branch, commit conventions, review flow
7. **[Uniflow Plugin Guide](../contributing/uniflow-plugin-guide.md)** — the most common substantial contribution, end to end

**You're done when:** you've built the platform from source, run the tests for the area you're changing, and opened a PR that follows the process.

---

Not sure which track fits? The [Getting Started overview](./index.md) has a lighter-weight chooser, and the [roadmap](./roadmap.md) shows where the platform is heading.
