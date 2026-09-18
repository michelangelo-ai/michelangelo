"""Convert a Kubeflow Trainer v2 TrainJob manifest into a Uniflow pipeline scaffold.

TrainJob is Kubeflow Trainer v2's replacement for the framework-specific v1
CRDs. Its nodes are homogeneous (no Master/Worker split), so the scaffold
sizes the Ray head and workers identically from ``trainer.resourcesPerNode``
and maps ``trainer.numNodes`` onto the worker count. Runtime plumbing with no
pipeline equivalent (dataset and model initializers, pod overrides, non-torch
runtimes) is surfaced as warnings and TODO comments rather than silently
dropped.
"""

import yaml

from michelangelo.cli.importer import scaffold
from michelangelo.cli.importer.base import ConversionResult, ManifestError

_KIND = "TrainJob"
_API_GROUP = "trainer.kubeflow.org"

# Keys the converter maps. Anything else in the corresponding block is
# reported as a warning so nothing is silently dropped.
_HANDLED_SPEC_FIELDS = frozenset({"trainer", "runtimeRef"})
_HANDLED_TRAINER_FIELDS = frozenset(
    {"image", "command", "args", "numNodes", "resourcesPerNode"}
)


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
    """Convert a TrainJob manifest dict into a pipeline scaffold.

    Raises :class:`ManifestError` when the manifest is not a TrainJob. A
    missing ``spec.trainer`` block is not an error: the runtime supplies
    defaults in Kubeflow, so the scaffold is emitted unsized with a warning.
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

    name = (manifest.get("metadata") or {}).get("name") or "imported-train-job"
    spec = manifest.get("spec") or {}

    _warn_unhandled(spec, _HANDLED_SPEC_FIELDS, "spec", warnings)

    runtime_name = (spec.get("runtimeRef") or {}).get("name") or ""
    if not runtime_name:
        warnings.append("spec.runtimeRef names no runtime; assuming a torch runtime")
    elif "torch" not in runtime_name:
        warnings.append(
            f"runtimeRef {runtime_name!r} is not a torch runtime; the scaffold"
            " uses LightningTrainer, which is PyTorch-only"
        )

    trainer = spec.get("trainer")
    if trainer is None:
        trainer = {}
        warnings.append(
            "no spec.trainer block: the runtime's defaults apply in Kubeflow;"
            " size the RayTask in the scaffold by hand"
        )
    else:
        _warn_unhandled(trainer, _HANDLED_TRAINER_FIELDS, "spec.trainer", warnings)

    num_nodes = int(trainer.get("numNodes") or 1)
    cpu, memory, gpu = scaffold.resources_from(trainer.get("resourcesPerNode"))

    # TrainJob nodes are homogeneous, so head and workers get the same shape.
    ray_task_fields = [
        ("head_cpu", cpu),
        ("head_memory", scaffold.quote(memory)),
        ("head_gpu", gpu or None),
        ("worker_cpu", cpu),
        ("worker_memory", scaffold.quote(memory)),
        ("worker_gpu", gpu or None),
        ("worker_instances", num_nodes),
    ]

    text = scaffold.TEMPLATE.format(
        source_kind=_KIND,
        source_name=name,
        ray_task_fields=scaffold.ray_task_lines(ray_task_fields),
        entrypoint=scaffold.entrypoint_comment(trainer, _KIND),
        run_name=name,
        num_workers=num_nodes,
        use_gpu=bool(gpu),
    )
    return ConversionResult(scaffold=text, warnings=warnings)


def _warn_unhandled(mapping, handled, label, warnings):
    """Warn once per key in ``mapping`` that the converter does not map."""
    for key in sorted(key for key in (mapping or {}) if key not in handled):
        warnings.append(
            f"{label}.{key} has no pipeline equivalent and was not converted"
        )
