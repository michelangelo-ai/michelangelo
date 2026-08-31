"""Convert a Kubeflow Trainer PyTorchJob manifest into a Uniflow pipeline scaffold.

The generated scaffold follows the shape documented in the Kubeflow Trainer
migration guide: a ``@uniflow.task`` sized by a ``RayTask`` config, running a
``LightningTrainer`` whose ``ScalingConfig`` mirrors the job's worker count.
Fields with no equivalent here (images, commands, volumes, scheduling hints)
are surfaced as warnings and TODO comments rather than silently dropped.
"""

import math

import yaml

from michelangelo.cli.importer.base import ConversionResult, ManifestError

_KIND = "PyTorchJob"
_API_GROUP = "kubeflow.org"

# PyTorchJob fields that have no direct equivalent in a pipeline task. Each is
# reported as a warning when present so nothing is silently dropped.
_UNMAPPED_SPEC_FIELDS = ("elasticPolicy", "runPolicy", "nprocPerNode")
_UNMAPPED_POD_FIELDS = (
    "volumes",
    "nodeSelector",
    "tolerations",
    "affinity",
    "initContainers",
)
_UNMAPPED_CONTAINER_FIELDS = ("env", "envFrom", "volumeMounts", "ports")


def convert_text(text: str) -> ConversionResult:
    """Parse a YAML manifest and convert it. See :func:`convert`."""
    try:
        manifest = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        raise ManifestError(f"input is not valid YAML: {exc}") from exc
    if not isinstance(manifest, dict):
        raise ManifestError(
            "input is not a Kubernetes manifest (expected a YAML mapping)"
        )
    return convert(manifest)


def convert(manifest: dict) -> ConversionResult:
    """Convert a PyTorchJob manifest dict into a pipeline scaffold.

    Raises :class:`ManifestError` when the manifest is not a PyTorchJob or has
    no replica specs at all.
    """
    warnings = []

    kind = manifest.get("kind")
    if kind != _KIND:
        raise ManifestError(
            f"unsupported kind {kind!r}: this converter handles {_KIND} manifests"
        )
    api_version = str(manifest.get("apiVersion") or "")
    if not api_version.startswith(_API_GROUP):
        warnings.append(
            f"apiVersion {api_version!r} is not from {_API_GROUP}; converting anyway"
        )

    name = (manifest.get("metadata") or {}).get("name") or "imported-pytorch-job"
    spec = manifest.get("spec") or {}

    for spec_field in _UNMAPPED_SPEC_FIELDS:
        if spec_field in spec:
            warnings.append(
                f"spec.{spec_field} has no pipeline equivalent and was not converted"
            )

    replica_specs = spec.get("pytorchReplicaSpecs") or {}
    master = replica_specs.get("Master")
    worker = replica_specs.get("Worker")
    if master is None and worker is None:
        raise ManifestError(
            "spec.pytorchReplicaSpecs has neither a Master nor a Worker block"
        )

    master_container = _first_container(master, "Master", warnings)
    worker_container = _first_container(worker, "Worker", warnings)

    if worker is not None:
        worker_replicas = int(worker.get("replicas") or 1)
    else:
        worker_replicas = 1
        warnings.append(
            "no Worker block: emitting a single-worker training group sized from Master"
        )
        worker_container = master_container

    head_cpu, head_memory, head_gpu = _container_resources(master_container)
    worker_cpu, worker_memory, worker_gpu = _container_resources(worker_container)

    _warn_unmapped_runtime(master_container, "Master", warnings)
    if worker_container is not master_container:
        _warn_unmapped_runtime(worker_container, "Worker", warnings)

    ray_task_fields = [
        ("head_cpu", head_cpu),
        ("head_memory", _quote(head_memory)),
        ("head_gpu", head_gpu or None),
        ("worker_cpu", worker_cpu),
        ("worker_memory", _quote(worker_memory)),
        ("worker_gpu", worker_gpu or None),
        ("worker_instances", worker_replicas),
    ]
    ray_task_lines = [f"        {k}={v}," for k, v in ray_task_fields if v is not None]
    if len(ray_task_lines) == 1:
        ray_task_lines.insert(0, "        # TODO: size the cluster for your workload.")

    entrypoint = _entrypoint_comment(worker_container or master_container)
    use_gpu = bool(worker_gpu)

    scaffold = _SCAFFOLD_TEMPLATE.format(
        source_name=name,
        ray_task_fields="\n".join(ray_task_lines),
        entrypoint=entrypoint,
        run_name=name,
        num_workers=worker_replicas,
        use_gpu=use_gpu,
    )
    return ConversionResult(scaffold=scaffold, warnings=warnings)


def _first_container(replica_spec, role, warnings):
    """Return the first container of a replica spec, warning about the rest."""
    if replica_spec is None:
        return None
    pod_spec = ((replica_spec.get("template") or {}).get("spec")) or {}
    for pod_field in _UNMAPPED_POD_FIELDS:
        if pod_field in pod_spec:
            warnings.append(
                f"{role} pod {pod_field} has no pipeline equivalent"
                " and was not converted"
            )
    if pod_spec.get("restartPolicy"):
        warnings.append(
            f"{role} restartPolicy was not converted:"
            " task retries are workflow-level here"
        )
    containers = pod_spec.get("containers") or []
    if not containers:
        warnings.append(
            f"{role} block has no containers; its resources were not converted"
        )
        return None
    if len(containers) > 1:
        warnings.append(
            f"{role} has {len(containers)} containers; only the first was converted"
        )
    return containers[0]


def _container_resources(container):
    """Extract (cpu, memory, gpu) from a container, preferring requests."""
    if container is None:
        return None, None, None
    resources = container.get("resources") or {}
    requests = resources.get("requests") or {}
    limits = resources.get("limits") or {}
    cpu = _cpu_count(requests.get("cpu", limits.get("cpu")))
    memory = requests.get("memory", limits.get("memory"))
    gpu = _gpu_count(limits.get("nvidia.com/gpu", requests.get("nvidia.com/gpu")))
    return cpu, memory, gpu


def _cpu_count(quantity):
    """Round a Kubernetes CPU quantity ("500m", "2", 2.5) up to whole CPUs."""
    if quantity is None:
        return None
    text = str(quantity)
    if text.endswith("m"):
        return math.ceil(int(text[:-1]) / 1000)
    return math.ceil(float(text))


def _gpu_count(quantity):
    if quantity is None:
        return None
    return int(str(quantity))


def _warn_unmapped_runtime(container, role, warnings):
    if container is None:
        return
    for container_field in _UNMAPPED_CONTAINER_FIELDS:
        if container_field in container:
            warnings.append(
                f"{role} container {container_field} has no pipeline equivalent"
                " and was not converted"
            )


def _entrypoint_comment(container):
    """Describe the container entrypoint the user must port into the task body."""
    lines = [
        "    # TODO: port the training code from your PyTorchJob image into this task."
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


def _quote(value):
    return None if value is None else repr(str(value))


_SCAFFOLD_TEMPLATE = '''"""Pipeline scaffold generated from PyTorchJob {source_name!r}.

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
