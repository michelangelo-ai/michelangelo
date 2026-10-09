"""Shared pipeline-scaffold generation for training-job converters."""

import math


def cpu_count(quantity):
    """Round a Kubernetes CPU quantity ("500m", "2", 2.5) up to whole CPUs."""
    if quantity is None:
        return None
    text = str(quantity)
    if text.endswith("m"):
        return math.ceil(int(text[:-1]) / 1000)
    return math.ceil(float(text))


def gpu_count(quantity):
    """Parse a Kubernetes GPU quantity into an int."""
    if quantity is None:
        return None
    return int(str(quantity))


def resources_from(requirements):
    """Extract (cpu, memory, gpu) from a ResourceRequirements dict.

    Requests are preferred over limits for cpu and memory; gpus are usually
    only set as limits, so limits are preferred there.
    """
    requirements = requirements or {}
    requests = requirements.get("requests") or {}
    limits = requirements.get("limits") or {}
    cpu = cpu_count(requests.get("cpu", limits.get("cpu")))
    memory = requests.get("memory", limits.get("memory"))
    gpu = gpu_count(limits.get("nvidia.com/gpu", requests.get("nvidia.com/gpu")))
    return cpu, memory, gpu


def quote(value):
    """Render a value as a Python string literal, passing None through."""
    return None if value is None else repr(str(value))


def ray_task_lines(fields):
    """Render RayTask keyword lines, dropping unset fields."""
    lines = [f"        {key}={value}," for key, value in fields if value is not None]
    if len(lines) == 1:
        lines.insert(0, "        # TODO: size the cluster for your workload.")
    return "\n".join(lines)


def entrypoint_comment(container, source_kind):
    """Describe the container entrypoint the user must port into the task body."""
    lines = [
        f"    # TODO: port the training code from your {source_kind} image"
        " into this task."
    ]
    if container:
        if container.get("image"):
            lines.append(f"    #   image:   {container['image']}")
        command = list(container.get("command") or []) + list(
            container.get("args") or []
        )
        if command:
            lines.append(f"    #   command: {' '.join(str(part) for part in command)}")
    return "\n".join(lines)


TEMPLATE = '''"""Pipeline scaffold generated from {source_kind} {source_name!r}.

Review every TODO before running: the converter maps cluster sizing, not
training code. See docs/user-guides/migration/migrate-from-kubeflow-trainer.md
for the full field mapping.
"""

import michelangelo.uniflow.core as uniflow
import ray.train
from michelangelo.lib.trainer.torch.pytorch_lightning import (
    LightningTrainer,
    LightningTrainerParam,
)
from michelangelo.uniflow.plugins.ray import RayTask


@uniflow.task(
    config=RayTask(
{ray_task_fields}
    )
)
def train(data_path: str):
{entrypoint}
    trainer = LightningTrainer(
        trainer_param=LightningTrainerParam(
            create_model_fn=create_model,  # TODO: your LightningModule factory
            create_model_fn_kwargs={{}},
            train_data=load_train(data_path),  # TODO: training Ray Dataset
            val_data=load_val(data_path),  # TODO: validation Ray Dataset
            batch_size=256,  # TODO: per-worker batch size
        ),
        run_config=ray.train.RunConfig(name={run_name!r}),
        scaling_config=ray.train.ScalingConfig(
            num_workers={num_workers},
            use_gpu={use_gpu},
        ),
    )
    return trainer.train()


@uniflow.workflow()
def training_pipeline(data_path: str):
    return train(data_path)
'''
