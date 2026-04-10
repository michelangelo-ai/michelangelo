# Model Training Guide

This guide explains **how to retrieve datasets for training** inside Michelangelo workflows and how to optionally scale training using **RayTask** and the **Lightning Trainer SDK**.

The focus is simplicity: **you control your training logic**, Michelangelo provides the dataset plumbing and optional distributed compute.

## What You'll Learn

* How datasets are passed to training tasks
* How to load Ray, Pandas, or Spark datasets
* How to scale training with Ray workers
* How to use the Lightning Trainer SDK for deep learning

## Prerequisites

- **A running sandbox** — Remote training runs require a local Kubernetes cluster. Follow the [Sandbox Setup](../getting-started/sandbox-setup.md) guide if you haven't done this yet.
- **A prepared dataset** — Training tasks expect datasets passed as `DatasetVariable`. See [Data Preparation](./prepare-your-data.md) for how to produce them.
- **Python 3.11+, Poetry, and the Michelangelo SDK installed** — Run `cd python && poetry install` from the repo root.
- **For distributed training:** A Docker image with your workflow code. See [Running Uniflow Pipelines](./ml-pipelines/running-uniflow.md) for image build steps.

## Understanding Training Inputs

Michelangelo workflows pass datasets using **DatasetVariable**.

A `DatasetVariable` may contain:

* **Ray Dataset** (recommended for distributed training)

* **Pandas DataFrame** (small/local datasets)

* **Spark DataFrame** (large-scale preprocessing)

Access the dataset inside a training task using:

```py
dataset = train_dv.value
```

### Dataset Formats

| Format | When It Appears | How to Use It |
| ----- | ----- | ----- |
| **Ray Dataset** | From data prep tasks using Ray | Best for distributed training |
| **Pandas DataFrame** | Local CSV or small data | Convert to tensors directly |
| **Spark DataFrame** | Spark preprocessing step | Convert to Pandas or Ray before training |

## Simple Training Example

For basic (scikit-learn, lightweight PyTorch) training, load your dataset directly:

```py
import michelangelo.uniflow.core as uniflow
from michelangelo.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="8Gi"))
def train_model(train_dv: DatasetVariable, val_dv: DatasetVariable):
    """Simple training with scikit-learn"""

    # Load datasets - works with Ray, Pandas, or Spark
    train_df = train_dv.value.to_pandas()
    val_df = val_dv.value.to_pandas()

    feature_cols = [col for col in train_df.columns if col != 'target']
    X_train, y_train = train_df[feature_cols], train_df['target']
    X_val, y_val = val_df[feature_cols], val_df['target']

    from sklearn.ensemble import RandomForestRegressor
    model = RandomForestRegressor(n_estimators=100)
    model.fit(X_train, y_train)

    score = model.score(X_val, y_val)
    print(f"Validation R² score: {score:.3f}")
    return model
```

## Distributed Training with Lightning Trainer SDK

To scale training across CPUs/GPUs, wrap your training task using **RayTask**.

## Example: Distributed Deep Learning with Ray Workers

```py
from michelangelo.lib.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer, LightningTrainerParam, create_run_config, create_scaling_config
)
from michelangelo.uniflow.plugins.ray import RayTask
from ray.train import CheckpointConfig

@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=4,
        worker_memory="16Gi",
        worker_instances=4,
    )
)
def train_distributed_model(
    train_dv: DatasetVariable,
    validation_dv: DatasetVariable,
    model_name: str = "gpt2",
    num_epochs: int = 3,
    batch_size: int = 32,
    learning_rate: float = 5e-5,
    num_workers: int = 4,
    use_gpu: bool = True,
):
    """Distributed training using Ray Lightning"""

    train_dv.load_ray_dataset()
    train_data = train_dv.value

    validation_dv.load_ray_dataset()
    validation_data = validation_dv.value

    # Scaling config
    scaling_config = create_scaling_config(
        trainer_cpu=2,
        cpu_per_worker=4,
        num_workers=num_workers,
        use_gpu=use_gpu,
    )

    # Run config with checkpointing
    run_config = create_run_config(
        name=f"distributed-training-{model_name}",
        checkpoint_config=CheckpointConfig(
            num_to_keep=1,
            checkpoint_score_attribute="val_loss",
            checkpoint_score_order="min",
        ),
    )

    # Lightning trainer parameters
    trainer_param = LightningTrainerParam(
        create_model=create_model_function,
        model_kwargs={
            "model_name": model_name,
            "learning_rate": learning_rate,
        },
        train_data=train_data,
        validation_data=validation_data,
        batch_size=batch_size,
        num_epochs=num_epochs,
        lightning_trainer_kwargs={
            "precision": "16-mixed",
            "log_every_n_steps": 10,
            "val_check_interval": 0.25,
        },
    )

    trainer = LightningTrainer(trainer_param)
    return trainer.train(run_config, scaling_config)
```

### What Ray Handles for You

* Worker creation  
* Dataset sharding  
* Parallel batch execution  
* GPU scheduling  
* Automatic checkpointing  
* Fault recovery

### Benefits of the Lightning Trainer SDK

| Benefit | Description |
| ----- | ----- |
| Automatic dataset sharding | No manual sampler or dataloader |
| Automatic distributed setup | Multi-node, multi-GPU ready |
| Automatic checkpointing | Lightning \+ model weights saved |
| Minimal boilerplate | Focus on model logic, not infrastructure |

You **do not** need to implement:

* dataloaders  
* DDP or multiprocessing

The SDK automates all distributed concerns.

## Best Practices

### Recommended

* Use trainer SDK for distributed deep learning  
* Start small, then scale  
* Track experiments consistently  
* Tune compute resources for your model

### Avoid

* Manual distributed loops unless necessary  
* Training without validation datasets  
* Ignoring memory/CPU/GPU limits

## Next Steps

Your models are now ready to move forward:

* Continue to [**Model Registry**](./model-registry-guide.md) to save and version
* Continue to **Model Deployment** _(Coming Soon)_ for inference

## Troubleshooting

* **Out of memory?** Lower batch size or increase memory  
* **Slow training?** Increase workers or enable GPU  
* **Loss not converging?** Verify preprocessing and learning rate