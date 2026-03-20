# Michelangelo User Guides

This guide provides you step by step how to build, train, and deploy machine learning models at scale using Michelangelo's unified ML platform.

## **Getting Started: Your First ML Workflow**

New to Michelangelo? Follow this step-by-step path to build your first end-to-end ML pipeline:

### **1. Understand ML Pipelines (Start Here)**
Before diving into specific tasks, understand how Michelangelo orchestrates ML workflows:
* [**ML Pipelines Overview**](./ml-pipelines/index.md) - Learn about tasks, workflows, and pipelines
* [**Getting Started with Pipelines**](./ml-pipelines/getting-started.md) - Build your first pipeline in 30 minutes

### **2. Prepare Your Data**
Get your data ready for training:
* [**Prepare Your Data**](./prepare-your-data.md) - Load, clean, and split datasets using Ray and Spark

### **3. Train Your Model**
Develop and train your ML model:
* [**Train and Register a Model**](./train-and-register-a-model.md) - Train locally or at scale with distributed computing

### **4. Manage Your Models**
Version and organize trained models:
* [**Model Registry Guide**](./model-registry-guide.md) - Version, track, and manage trained models

### **5. Deploy Your Model**
Serve predictions in production:
* **Deploy Models** _(Coming Soon)_ - Deploy models for real-time inference and batch scoring

---

## **Quick Navigation**

### **Core Concepts**

| Concept | Learn About It |
| ----- | ----- |
| **How to build a pipeline** | [ML Pipelines Overview](./ml-pipelines/index.md) |
| **When to use each execution mode** | [Pipeline Running Modes](./ml-pipelines/pipeline-running-modes.md) |
| **How caching and resumability work** | [Caching and Resume](./ml-pipelines/cache-and-pipelinerun-resume-form.md) |
| **How to iterate rapidly** | [File Sync Testing](./ml-pipelines/file-sync-testing-flow-runbook.md) |

### **Specific Tasks**

* [Prepare your data](./prepare-your-data.md)
* [Train a model](./train-and-register-a-model.md)
* [Manage your models](./model-registry-guide.md)
* Deploy models _(Coming Soon)_

### **Advanced Topics**

* [Set Up Pipeline Triggers](./set-up-triggers.md) - Schedule pipelines to run automatically
* [CLI Reference](./cli.md) - Command-line tools for pipeline and project management
* [Project Management](./project-management-for-ml-pipelines.md) - Create and manage MA Studio projects

---

## **Learning by Examples**

Choose a tutorial based on your ML domain:

### **Traditional Machine Learning**

| Example | Description | Techniques |
| ----- | ----- | ----- |
| [**Boston Housing Regression**](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/boston_housing_xgb) | Predict house prices using tabular data with XGBoost | Feature engineering, distributed training |

### **Natural Language Processing**

| Example | Description | Techniques |
| ----- | ----- | ----- |
| [**BERT Text Classification**](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/bert_cola) | Classify text using pre-trained transformer models | Fine-tuning, distributed GPU training |
| [**GPT Fine-tuning**](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/gpt_oss_20b_finetune) | Train large language models with LoRA adapters | Memory optimization, multi-GPU scaling |

### **Recommendation Systems**

| Example | Description | Techniques |
| ----- | ----- | ----- |
| [**Amazon Books Recommendation**](https://github.com/michelangelo-ai/michelangelo/tree/main/python/examples/amazon_books_qwen) | Build dual-encoder recommendation system | Embedding learning, similarity search |

---

## What's next?

- **New to ML Pipelines?** Start with the [ML Pipelines Overview](./ml-pipelines/index.md), then follow [Getting Started with Pipelines](./ml-pipelines/getting-started.md)
- **Have your pipeline running?** Learn about [Pipeline Running Modes](./ml-pipelines/pipeline-running-modes.md) and [Caching and Resume](./ml-pipelines/cache-and-pipelinerun-resume-form.md)
- **Ready for production?** Set up [Pipeline Triggers](./set-up-triggers.md) for automated scheduling
