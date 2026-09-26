"""Tests for the RayJob-to-michelangelo.api/v2 converter."""

import pytest
import yaml

from michelangelo.cli.importer import rayjob

_SELECTOR_MANIFEST = """
apiVersion: ray.io/v1
kind: RayJob
metadata:
  name: sample-job
  namespace: team-a
spec:
  entrypoint: python sample.py
  jobId: sample-123
  clusterSelector:
    ray.io/cluster: shared-cluster
"""

_INLINE_MANIFEST = """
apiVersion: ray.io/v1
kind: RayJob
metadata:
  name: train-job
spec:
  entrypoint: python train.py
  rayClusterSpec:
    rayVersion: 2.9.0
    headGroupSpec:
      serviceType: ClusterIP
      rayStartParams:
        dashboard-host: 0.0.0.0
      template:
        spec:
          containers:
            - name: ray-head
              image: rayproject/ray:2.9.0
    workerGroupSpecs:
      - groupName: gpu-workers
        replicas: 3
        minReplicas: 1
        maxReplicas: 5
        rayStartParams: {}
        template:
          spec:
            containers:
              - name: ray-worker
                image: rayproject/ray:2.9.0
"""


def _docs(result):
    return list(yaml.safe_load_all(result.scaffold))


def test_cluster_selector_emits_single_rayjob():
    """Cluster selector emits single rayjob."""
    result = rayjob.convert_text(_SELECTOR_MANIFEST)
    docs = _docs(result)
    assert len(docs) == 1
    job = docs[0]
    assert job["apiVersion"] == "michelangelo.api/v2"
    assert job["kind"] == "RayJob"
    assert job["metadata"] == {"name": "sample-job", "namespace": "team-a"}
    assert job["spec"]["entrypoint"] == "python sample.py"
    assert job["spec"]["jobId"] == "sample-123"
    assert job["spec"]["cluster"] == {"name": "shared-cluster", "namespace": "team-a"}
    assert job["spec"]["user"]["name"].startswith("TODO")
    # The only warning is the always-present user identity reminder.
    assert len(result.warnings) == 1
    assert "user identity" in result.warnings[0]


def test_inline_cluster_emits_cluster_and_job():
    """Inline cluster emits cluster and job."""
    result = rayjob.convert_text(_INLINE_MANIFEST)
    docs = _docs(result)
    assert len(docs) == 2
    cluster, job = docs
    assert cluster["kind"] == "RayCluster"
    assert cluster["apiVersion"] == "michelangelo.api/v2"
    assert cluster["metadata"] == {"name": "train-job"}
    assert cluster["spec"]["rayVersion"] == "2.9.0"
    head = cluster["spec"]["head"]
    assert head["serviceType"] == "ClusterIP"
    assert head["rayStartParams"] == {"dashboard-host": "0.0.0.0"}
    assert head["pod"]["spec"]["containers"][0]["name"] == "ray-head"
    (worker,) = cluster["spec"]["workers"]
    assert worker["nodeType"] == "gpu-workers"
    assert worker["minInstances"] == 1
    assert worker["maxInstances"] == 5
    assert worker["pod"]["spec"]["containers"][0]["name"] == "ray-worker"
    assert "rayStartParams" not in worker  # empty map is dropped
    assert job["kind"] == "RayJob"
    assert job["spec"]["cluster"] == {"name": "train-job"}
    assert job["spec"]["entrypoint"] == "python train.py"
    assert "jobId" not in job["spec"]
    assert len(result.warnings) == 1  # only the user identity reminder


def test_worker_replicas_fallback():
    """Worker replicas fallback."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    workers = manifest["spec"]["rayClusterSpec"]["workerGroupSpecs"]
    workers[0] = {
        "groupName": "cpu",
        "replicas": 4,
        "rayStartParams": {"num-cpus": "4"},
    }
    result = rayjob.convert(manifest)
    (worker,) = _docs(result)[0]["spec"]["workers"]
    assert worker["minInstances"] == 4
    assert worker["maxInstances"] == 4
    assert worker["rayStartParams"] == {"num-cpus": "4"}


def test_worker_max_only_fallback():
    """Worker max only fallback."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    manifest["spec"]["rayClusterSpec"]["workerGroupSpecs"] = [
        {"groupName": "cpu", "maxReplicas": 6}
    ]
    result = rayjob.convert(manifest)
    (worker,) = _docs(result)[0]["spec"]["workers"]
    assert worker["minInstances"] == 6
    assert worker["maxInstances"] == 6


def test_worker_without_counts_defaults_to_one():
    """Worker without counts defaults to one."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    manifest["spec"]["rayClusterSpec"]["workerGroupSpecs"] = [{"groupName": "cpu"}]
    result = rayjob.convert(manifest)
    (worker,) = _docs(result)[0]["spec"]["workers"]
    assert worker["minInstances"] == 1
    assert worker["maxInstances"] == 1
    assert any("no replica counts" in w for w in result.warnings)


def test_worker_without_group_name_warns():
    """Worker without group name warns."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    manifest["spec"]["rayClusterSpec"]["workerGroupSpecs"] = [{"replicas": 2}]
    result = rayjob.convert(manifest)
    (worker,) = _docs(result)[0]["spec"]["workers"]
    assert "nodeType" not in worker
    assert any("no groupName" in w for w in result.warnings)


def test_wrong_kind_raises():
    """Wrong kind raises."""
    with pytest.raises(rayjob.ManifestError, match="unsupported kind 'RayService'"):
        rayjob.convert({"kind": "RayService"})


def test_non_ray_api_group_warns():
    """Non ray api group warns."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["apiVersion"] = "example.com/v1"
    result = rayjob.convert(manifest)
    assert any("is not from ray.io" in w for w in result.warnings)


def test_invalid_yaml_raises():
    """Invalid yaml raises."""
    with pytest.raises(rayjob.ManifestError, match="not valid YAML"):
        rayjob.convert_text("kind: [broken")


def test_non_mapping_input_raises():
    """Non mapping input raises."""
    with pytest.raises(rayjob.ManifestError, match="expected a YAML mapping"):
        rayjob.convert_text("- a list")


def test_no_cluster_at_all_raises():
    """No cluster at all raises."""
    with pytest.raises(rayjob.ManifestError, match="neither clusterSelector"):
        rayjob.convert({"kind": "RayJob", "spec": {"entrypoint": "python x.py"}})


def test_selector_wins_over_inline_spec():
    """Selector wins over inline spec."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["spec"]["rayClusterSpec"] = {"rayVersion": "2.9.0"}
    result = rayjob.convert(manifest)
    docs = _docs(result)
    assert len(docs) == 1  # no RayCluster emitted
    assert docs[0]["spec"]["cluster"]["name"] == "shared-cluster"
    assert any("clusterSelector wins" in w for w in result.warnings)


def test_selector_without_canonical_label():
    """Selector without canonical label."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["spec"]["clusterSelector"] = {"env": "prod"}
    result = rayjob.convert(manifest)
    job = _docs(result)[0]
    assert job["spec"]["cluster"]["name"].startswith("TODO")
    assert any("no 'ray.io/cluster' label" in w for w in result.warnings)
    assert any("labels ['env']" in w for w in result.warnings)


def test_selector_extra_labels_warn():
    """Selector extra labels warn."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["spec"]["clusterSelector"]["team"] = "ml"
    result = rayjob.convert(manifest)
    job = _docs(result)[0]
    assert job["spec"]["cluster"]["name"] == "shared-cluster"
    assert any("labels ['team']" in w for w in result.warnings)


def test_missing_entrypoint_leaves_todo():
    """Missing entrypoint leaves todo."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    del manifest["spec"]["entrypoint"]
    result = rayjob.convert(manifest)
    job = _docs(result)[0]
    assert job["spec"]["entrypoint"].startswith("TODO")
    assert any("entrypoint is missing" in w for w in result.warnings)


def test_unmapped_spec_fields_warn():
    """Unmapped spec fields warn."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["spec"]["shutdownAfterJobFinishes"] = True
    manifest["spec"]["ttlSecondsAfterFinished"] = 60
    manifest["spec"]["runtimeEnvYAML"] = "pip: [torch]"
    result = rayjob.convert(manifest)
    for field in (
        "shutdownAfterJobFinishes",
        "ttlSecondsAfterFinished",
        "runtimeEnvYAML",
    ):
        assert any(f"spec.{field} has no v2 equivalent" in w for w in result.warnings)


def test_unmapped_cluster_head_and_worker_fields_warn():
    """Unmapped cluster head and worker fields warn."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    cluster = manifest["spec"]["rayClusterSpec"]
    cluster["enableInTreeAutoscaling"] = True
    cluster["headGroupSpec"]["enableIngress"] = True
    cluster["workerGroupSpecs"][0]["numOfHosts"] = 2
    result = rayjob.convert(manifest)
    assert any(
        "spec.rayClusterSpec.enableInTreeAutoscaling" in w for w in result.warnings
    )
    assert any("headGroupSpec.enableIngress" in w for w in result.warnings)
    assert any("workerGroupSpecs[0].numOfHosts" in w for w in result.warnings)


def test_missing_head_group_warns():
    """Missing head group warns."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    del manifest["spec"]["rayClusterSpec"]["headGroupSpec"]
    result = rayjob.convert(manifest)
    cluster = _docs(result)[0]
    assert "head" not in cluster["spec"]
    assert any("no headGroupSpec" in w for w in result.warnings)


def test_minimal_inline_cluster():
    """Minimal inline cluster."""
    result = rayjob.convert(
        {
            "apiVersion": "ray.io/v1",
            "kind": "RayJob",
            "spec": {"rayClusterSpec": {"headGroupSpec": {}}},
        }
    )
    cluster, job = _docs(result)
    assert cluster["metadata"] == {"name": "imported-ray-job"}
    assert "rayVersion" not in cluster["spec"]
    assert cluster["spec"]["head"] == {}
    assert "workers" not in cluster["spec"]
    assert job["spec"]["cluster"] == {"name": "imported-ray-job"}


def test_namespace_flows_to_cluster_and_reference():
    """Namespace flows to cluster and reference."""
    manifest = yaml.safe_load(_INLINE_MANIFEST)
    manifest["metadata"]["namespace"] = "ml-team"
    result = rayjob.convert(manifest)
    cluster, job = _docs(result)
    assert cluster["metadata"]["namespace"] == "ml-team"
    assert job["metadata"]["namespace"] == "ml-team"
    assert job["spec"]["cluster"] == {"name": "train-job", "namespace": "ml-team"}


def test_labels_and_annotations_pass_through():
    """Labels and annotations pass through."""
    manifest = yaml.safe_load(_SELECTOR_MANIFEST)
    manifest["metadata"]["labels"] = {"app": "demo"}
    manifest["metadata"]["annotations"] = {"note": "hi"}
    result = rayjob.convert(manifest)
    job = _docs(result)[0]
    assert job["metadata"]["labels"] == {"app": "demo"}
    assert job["metadata"]["annotations"] == {"note": "hi"}
