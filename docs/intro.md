---
slug: /
sidebar_position: 1
title: Welcome
---

# Welcome to Michelangelo

Michelangelo is an end-to-end ML platform for building, deploying, and managing machine learning models. Born at Uber — where it powers **25,000+ model trainings per month** and **~30 million predictions per second** — now open source.

## Get started

### I'm evaluating Michelangelo

Understand what the platform does, how it compares to your current stack, and whether it fits your use case.

- **[Overview](/docs/about/overview)** — What Michelangelo is, how it works, and how familiar tools map to it
- **[Core Concepts](/docs/about/core-concepts-and-key-terms)** — Projects, workflows, tasks, and the key terms you'll encounter

### I want to build my first pipeline

Get a local environment running and build an end-to-end ML pipeline.

- **[Sandbox Setup](/docs/setup-guide/sandbox-setup)** — Set up a local Michelangelo cluster (~20 min)
- **[Getting Started with Pipelines](/docs/user-guides/ml-pipelines/getting-started)** — Build your first pipeline from scratch (~30 min)
- **[Example Projects](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples)** — Boston Housing, BERT text classification, GPT fine-tuning, and more

### I'm deploying or operating the platform

Set up infrastructure, configure compute clusters, and deploy the UI.

- **[Operator Guides](/docs/operator-guides)** — API framework, compute clusters, and serving infrastructure
- **[Building from Source](/docs/contributing/building-michelangelo-ai-from-source)** — Compile and run the platform locally

### I want to contribute

- **[Developer Guide](/docs/contributing/developer-guide)** — Contribution workflow and development practices
- **[Documentation Guide](/docs/contributing/documentation-guide)** — How to write and structure docs

## What Michelangelo is — and isn't

Understanding scope helps you decide if Michelangelo is the right tool.

**Michelangelo is:**
- An **ML lifecycle platform** — data prep, training, evaluation, deployment, and monitoring in one system
- An **orchestration framework** (Uniflow) for writing ML pipelines as Python code with `@task` and `@workflow` decorators
- A **model registry** for versioning, tracking, and managing trained models
- A **deployment system** for online inference (Triton) and batch predictions
- A **no-code UI** (MA Studio) for standard ML workflows without writing code

**Michelangelo is not:**
- A **notebook environment** — use Jupyter/Colab for exploration, then bring your code to Michelangelo for production
- A **data warehouse** — it connects to your existing data sources (S3, Snowflake, BigQuery, HDFS)
- A **general-purpose workflow engine** — it's purpose-built for ML, not arbitrary DAGs
- A **model monitoring SaaS** — monitoring is built in, but Michelangelo is self-hosted infrastructure, not a managed service
- A **replacement for your ML framework** — use PyTorch, TensorFlow, XGBoost, scikit-learn as you normally would

## Quick links

- [GitHub Repository](https://github.com/michelangelo-ai/michelangelo)
- [CLI Reference](/docs/user-guides/cli)
- [ML Pipelines Overview](/docs/user-guides/ml-pipelines)
