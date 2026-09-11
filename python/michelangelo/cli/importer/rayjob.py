"""Convert a KubeRay RayJob manifest into michelangelo.api/v2 resources.

Follows the mapping documented in the KubeRay migration guide: a RayJob that
targets an existing cluster via ``clusterSelector`` becomes a single
michelangelo.api/v2 RayJob referencing that cluster; a RayJob carrying an
inline ``rayClusterSpec`` becomes a v2 RayCluster plus a v2 RayJob that
references it. Fields with no v2 equivalent (shutdown and TTL policies,
submitter pod templates, runtime envs, autoscaler options) are surfaced as
warnings rather than silently dropped.
"""

import yaml

from michelangelo.cli.importer.base import ConversionResult, ManifestError

_KIND = "RayJob"
_API_GROUP = "ray.io"

# The canonical KubeRay label used by clusterSelector to target a cluster.
_CLUSTER_SELECTOR_KEY = "ray.io/cluster"

_USER_TODO = "TODO-your-username"
_ENTRYPOINT_TODO = "TODO: python your_script.py"

# Keys the converter maps. Anything else in the corresponding block is
# reported as a warning so nothing is silently dropped.
_HANDLED_SPEC_FIELDS = frozenset(
    {"entrypoint", "jobId", "clusterSelector", "rayClusterSpec"}
)
_HANDLED_CLUSTER_FIELDS = frozenset({"rayVersion", "headGroupSpec", "workerGroupSpecs"})
_HANDLED_HEAD_FIELDS = frozenset({"serviceType", "rayStartParams", "template"})
_HANDLED_WORKER_FIELDS = frozenset(
    {
        "groupName",
        "replicas",
        "minReplicas",
        "maxReplicas",
        "rayStartParams",
        "template",
    }
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
    """Convert a KubeRay RayJob dict into michelangelo.api/v2 manifests.

    Raises :class:`ManifestError` when the manifest is not a RayJob or names
    no cluster at all (neither ``clusterSelector`` nor ``rayClusterSpec``).
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

    metadata = manifest.get("metadata") or {}
    name = metadata.get("name") or "imported-ray-job"
    namespace = metadata.get("namespace")
    spec = manifest.get("spec") or {}

    _warn_unhandled(spec, _HANDLED_SPEC_FIELDS, "spec", warnings)

    selector = spec.get("clusterSelector")
    cluster_spec = spec.get("rayClusterSpec")
    if selector is None and cluster_spec is None:
        raise ManifestError(
            "spec has neither clusterSelector nor rayClusterSpec;"
            " there is no cluster to convert or reference"
        )

    warnings.append(
        "KubeRay manifests carry no user identity;"
        " replace the TODO in spec.user.name before applying"
    )

    documents = []
    if selector is not None:
        if cluster_spec is not None:
            warnings.append(
                "both clusterSelector and rayClusterSpec are set; clusterSelector"
                " wins (as in KubeRay) and the inline spec was not converted"
            )
        cluster_ref = _cluster_from_selector(selector, namespace, warnings)
    else:
        documents.append(_cluster_manifest(name, namespace, cluster_spec, warnings))
        cluster_ref = {"name": name}
        if namespace:
            cluster_ref["namespace"] = namespace

    documents.append(_job_manifest(name, metadata, spec, cluster_ref, warnings))

    text = yaml.safe_dump_all(documents, sort_keys=False, default_flow_style=False)
    return ConversionResult(scaffold=text, warnings=warnings)


def _cluster_from_selector(selector, namespace, warnings):
    """Build the v2 cluster reference from a KubeRay clusterSelector."""
    cluster_name = (selector or {}).get(_CLUSTER_SELECTOR_KEY)
    if not cluster_name:
        cluster_name = "TODO-cluster-name"
        warnings.append(
            f"clusterSelector has no {_CLUSTER_SELECTOR_KEY!r} label;"
            " set spec.cluster.name to the target cluster"
        )
    extra = sorted(key for key in (selector or {}) if key != _CLUSTER_SELECTOR_KEY)
    if extra:
        warnings.append(
            f"clusterSelector labels {extra} were not converted;"
            " v2 references clusters by name"
        )
    cluster_ref = {"name": cluster_name}
    if namespace:
        cluster_ref["namespace"] = namespace
    return cluster_ref


def _cluster_manifest(name, namespace, cluster_spec, warnings):
    """Convert an inline rayClusterSpec into a v2 RayCluster manifest."""
    _warn_unhandled(
        cluster_spec, _HANDLED_CLUSTER_FIELDS, "spec.rayClusterSpec", warnings
    )

    spec = {"user": {"name": _USER_TODO}}
    if cluster_spec.get("rayVersion"):
        spec["rayVersion"] = cluster_spec["rayVersion"]

    head = cluster_spec.get("headGroupSpec")
    if head is None:
        warnings.append(
            "rayClusterSpec has no headGroupSpec;"
            " add spec.head to the generated RayCluster"
        )
    else:
        spec["head"] = _head(head, warnings)

    workers = cluster_spec.get("workerGroupSpecs") or []
    if workers:
        spec["workers"] = [
            _worker(worker, index, warnings) for index, worker in enumerate(workers)
        ]

    out_metadata = {"name": name}
    if namespace:
        out_metadata["namespace"] = namespace
    return {
        "apiVersion": "michelangelo.api/v2",
        "kind": "RayCluster",
        "metadata": out_metadata,
        "spec": spec,
    }


def _head(head, warnings):
    """Convert a KubeRay headGroupSpec into a v2 head spec."""
    _warn_unhandled(head, _HANDLED_HEAD_FIELDS, "headGroupSpec", warnings)
    out = {}
    if head.get("serviceType"):
        out["serviceType"] = head["serviceType"]
    if head.get("template") is not None:
        out["pod"] = head["template"]
    if head.get("rayStartParams"):
        out["rayStartParams"] = head["rayStartParams"]
    return out


def _worker(worker, index, warnings):
    """Convert one KubeRay workerGroupSpec into a v2 worker spec."""
    label = f"workerGroupSpecs[{index}]"
    _warn_unhandled(worker, _HANDLED_WORKER_FIELDS, label, warnings)

    out = {}
    group_name = worker.get("groupName")
    if group_name:
        out["nodeType"] = group_name
    else:
        warnings.append(f"{label} has no groupName; set nodeType on the output")

    counts = [
        worker.get("minReplicas"),
        worker.get("replicas"),
        worker.get("maxReplicas"),
    ]
    min_instances = next((count for count in counts if count is not None), None)
    max_instances = next(
        (count for count in reversed(counts) if count is not None), None
    )
    if min_instances is None:
        min_instances = max_instances = 1
        warnings.append(f"{label} has no replica counts; defaulting to one instance")
    out["minInstances"] = int(min_instances)
    out["maxInstances"] = int(max_instances)

    if worker.get("template") is not None:
        out["pod"] = worker["template"]
    if worker.get("rayStartParams"):
        out["rayStartParams"] = worker["rayStartParams"]
    return out


def _job_manifest(name, metadata, spec, cluster_ref, warnings):
    """Build the v2 RayJob manifest referencing ``cluster_ref``."""
    entrypoint = spec.get("entrypoint")
    if not entrypoint:
        entrypoint = _ENTRYPOINT_TODO
        warnings.append("spec.entrypoint is missing; fill in the TODO in the output")

    job_spec = {
        "user": {"name": _USER_TODO},
        "entrypoint": entrypoint,
        "cluster": cluster_ref,
    }
    if spec.get("jobId"):
        job_spec["jobId"] = spec["jobId"]

    out_metadata = {"name": name}
    if metadata.get("namespace"):
        out_metadata["namespace"] = metadata["namespace"]
    for passthrough in ("labels", "annotations"):
        if metadata.get(passthrough):
            out_metadata[passthrough] = metadata[passthrough]
    return {
        "apiVersion": "michelangelo.api/v2",
        "kind": "RayJob",
        "metadata": out_metadata,
        "spec": job_spec,
    }


def _warn_unhandled(mapping, handled, label, warnings):
    """Warn once per key in ``mapping`` that the converter does not map."""
    for key in sorted(key for key in (mapping or {}) if key not in handled):
        warnings.append(f"{label}.{key} has no v2 equivalent and was not converted")
