# Boston Housing XGBoost Demo

Regression model demo using XGBoost to predict Boston housing prices. Demonstrates hybrid workflow with Spark preprocessing and Ray-based distributed training.

## Features

- **Classic Dataset**: Boston housing price prediction
- **Hybrid Execution**: Spark for preprocessing, Ray for training
- **XGBoost Training**: Distributed gradient boosting with Ray
- **Auto-scaling**: Dynamic worker allocation based on cluster resources
- **Data Pipeline**: Feature preparation, type casting, and train/validation split

## How to Run

```bash
cd /Users/sally.lee/Uber/michelangelo-ai/michelangelo/python
source .venv/bin/activate
PYTHONPATH=. poetry run python examples/boston_housing_xgb/boston_housing_xgb.py
```

## Expected Output

```
Feature preparation completed
Train dataset schema: ...
Preprocessing with Spark...
Training XGBoost model with Ray...
[0] train-rmse:4.5123
[1] train-rmse:3.2156
...
[9] train-rmse:1.8234
train_result.path: /tmp/ray_results/...
```
