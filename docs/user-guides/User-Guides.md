# Michelangelo User Guides 


This guide provides you step by step how to build, train, and deploy machine learning models at scale using Michelangelo's unified ML platform.

## **Get Started**

New to Michelangelo? Start here to learn the complete ML workflow:

### **Core Tutorials**

| Tutorial | Description |
| ----- | ----- |
| **Data Preparation** | Transform and prepare datasets using Ray and Spark distributed processing. Load CSVs, clean data, and create train/validation splits. |
| **Model Training** | Train models locally or at scale with distributed computing. Use scikit-learn for simple cases or Lightning Trainer SDK for deep learning. |
| **Model Registry** | Version, track, and manage trained models with MLflow integration. Organize experiments and model artifacts. |
| **Model Deployment** | Deploy models for real-time inference and batch scoring. Set up REST endpoints and monitor production models. |

## **Quick Navigation**

Choose your path based on your current goal:

### **Specific Tasks**

* [Prepare your data](https://github.com/michelangelo-ai/michelangelo/wiki/Prepare-Your-Data)
* [Train a model](https://github.com/michelangelo-ai/michelangelo/wiki/Train-and-Register-a-Model)
* [Mode registry guide](https://github.com/michelangelo-ai/michelangelo/wiki/Model-Registry-Guide)
* Deploy models _(Coming Soon)_

## **What You'll Learn**

By the end of these tutorials, you'll be able to:

* Prepare datasets efficiently using Ray and Spark  
* Train models both locally and with distributed computing  
* Register models using mode package SDK  
* Deploy models for real-time serving

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