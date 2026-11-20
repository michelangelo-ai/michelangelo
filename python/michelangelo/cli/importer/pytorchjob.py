"""Convert a Kubeflow Trainer PyTorchJob manifest into a Uniflow pipeline scaffold.

The generated scaffold follows the shape documented in the Kubeflow Trainer
migration guide: a ``@uniflow.task`` sized by a ``RayTask`` config, running a
``LightningTrainer`` whose ``ScalingConfig`` mirrors the job's worker count.
Fields with no equivalent here (images, commands, volumes, scheduling hints)
are surfaced as warnings and TODO comments rather than silently dropped.
"""

import yaml

from michelangelo.cli.importer import scaffold
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
        ("head_memory", scaffold.quote(head_memory)),
        ("head_gpu", head_gpu or None),
        ("worker_cpu", worker_cpu),
        ("worker_memory", scaffold.quote(worker_memory)),
        ("worker_gpu", worker_gpu or None),
        ("worker_instances", worker_replicas),
    ]

    text = scaffold.TEMPLATE.format(
        source_kind=_KIND,
        source_name=name,
        ray_task_fields=scaffold.ray_task_lines(ray_task_fields),
        entrypoint=scaffold.entrypoint_comment(
            worker_container or master_container, _KIND
        ),
        run_name=name,
        num_workers=worker_replicas,
        use_gpu=bool(worker_gpu),
    )
    return ConversionResult(scaffold=text, warnings=warnings)


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
    return scaffold.resources_from(container.get("resources"))


def _warn_unmapped_runtime(container, role, warnings):
    if container is None:
        return
    for container_field in _UNMAPPED_CONTAINER_FIELDS:
        if container_field in container:
            warnings.append(
                f"{role} container {container_field} has no pipeline equivalent"
                " and was not converted"
            )
