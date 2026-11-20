"""Tests for the PyTorchJob-to-pipeline converter."""

import pytest

from michelangelo.cli.importer import pytorchjob

_FULL_MANIFEST = """
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: pytorch-dist-train
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      template:
        spec:
          containers:
            - name: pytorch
              image: my-training-image:latest
              command: ["python", "train.py"]
              resources:
                requests:
                  cpu: "2"
                  memory: 4Gi
    Worker:
      replicas: 3
      template:
        spec:
          containers:
            - name: pytorch
              image: my-training-image:latest
              command: ["python"]
              args: ["train.py"]
              resources:
                requests:
                  cpu: 500m
                  memory: 16Gi
                limits:
                  nvidia.com/gpu: 1
"""


def test_full_manifest_maps_resources_and_scaling():
    """Full manifest maps resources and scaling."""
    result = pytorchjob.convert_text(_FULL_MANIFEST)
    scaffold = result.scaffold
    assert "head_cpu=2," in scaffold
    assert "head_memory='4Gi'," in scaffold
    assert "worker_cpu=1," in scaffold  # 500m rounds up to a whole CPU
    assert "worker_memory='16Gi'," in scaffold
    assert "worker_gpu=1," in scaffold
    assert "worker_instances=3," in scaffold
    assert "num_workers=3," in scaffold
    assert "use_gpu=True," in scaffold
    assert "name='pytorch-dist-train'" in scaffold
    # Entrypoint surfaced as TODO comments, with args appended to command.
    assert "image:   my-training-image:latest" in scaffold
    assert "command: python train.py" in scaffold
    assert result.warnings == []


def test_scaffold_is_valid_python():
    """Scaffold is valid python."""
    result = pytorchjob.convert_text(_FULL_MANIFEST)
    compile(result.scaffold, "<generated>", "exec")


def test_wrong_kind_is_rejected():
    """Wrong kind is rejected."""
    with pytest.raises(pytorchjob.ManifestError, match="unsupported kind 'TFJob'"):
        pytorchjob.convert_text("kind: TFJob\napiVersion: kubeflow.org/v1\n")


def test_invalid_yaml_is_rejected():
    """Invalid yaml is rejected."""
    with pytest.raises(pytorchjob.ManifestError, match="not valid YAML"):
        pytorchjob.convert_text("kind: [unclosed")


def test_non_mapping_input_is_rejected():
    """Non mapping input is rejected."""
    with pytest.raises(pytorchjob.ManifestError, match="expected a YAML mapping"):
        pytorchjob.convert_text("- just\n- a\n- list\n")


def test_missing_replica_specs_is_rejected():
    """Missing replica specs is rejected."""
    manifest = "kind: PyTorchJob\napiVersion: kubeflow.org/v1\nspec: {}\n"
    with pytest.raises(pytorchjob.ManifestError, match="neither a Master nor a Worker"):
        pytorchjob.convert_text(manifest)


def test_master_only_manifest_warns_and_sizes_from_master():
    """Master only manifest warns and sizes from master."""
    manifest = """
apiVersion: kubeflow.org/v1
kind: PyTorchJob
spec:
  pytorchReplicaSpecs:
    Master:
      replicas: 1
      template:
        spec:
          containers:
            - name: pytorch
              resources:
                limits:
                  cpu: 2500m
                  memory: 8Gi
"""
    result = pytorchjob.convert_text(manifest)
    assert "worker_instances=1," in result.scaffold
    assert "worker_cpu=3," in result.scaffold  # 2500m rounds up, limits fallback
    assert "worker_memory='8Gi'," in result.scaffold
    assert "num_workers=1," in result.scaffold
    assert "use_gpu=False," in result.scaffold
    assert "name='imported-pytorch-job'" in result.scaffold
    assert any("no Worker block" in w for w in result.warnings)


def test_unmapped_fields_produce_warnings_not_silence():
    """Unmapped fields produce warnings not silence."""
    manifest = """
apiVersion: training.example.io/v1
kind: PyTorchJob
metadata:
  name: busy-job
spec:
  nprocPerNode: "2"
  elasticPolicy: {minReplicas: 1}
  runPolicy: {cleanPodPolicy: Running}
  pytorchReplicaSpecs:
    Worker:
      replicas: 2
      template:
        spec:
          restartPolicy: OnFailure
          nodeSelector: {pool: gpu}
          volumes: [{name: data}]
          containers:
            - name: pytorch
              env: [{name: FOO, value: bar}]
              volumeMounts: [{name: data, mountPath: /data}]
            - name: sidecar
"""
    result = pytorchjob.convert_text(manifest)
    joined = "\n".join(result.warnings)
    for expected in (
        "apiVersion 'training.example.io/v1'",
        "spec.nprocPerNode",
        "spec.elasticPolicy",
        "spec.runPolicy",
        "restartPolicy",
        "nodeSelector",
        "volumes",
        "env",
        "volumeMounts",
        "2 containers; only the first",
    ):
        assert expected in joined, f"missing warning about {expected}"


def test_containerless_worker_warns_and_emits_sizing_todo():
    """Containerless worker warns and emits sizing todo."""
    manifest = """
apiVersion: kubeflow.org/v1
kind: PyTorchJob
spec:
  pytorchReplicaSpecs:
    Worker:
      replicas: 2
      template:
        spec: {}
"""
    result = pytorchjob.convert_text(manifest)
    assert any("no containers" in w for w in result.warnings)
    assert "# TODO: size the cluster for your workload." in result.scaffold
    assert "worker_instances=2," in result.scaffold
