"""Tests for the TrainJob-to-pipeline converter."""

import pytest
import yaml

from michelangelo.cli.importer import trainjob

_FULL_MANIFEST = """
apiVersion: trainer.kubeflow.org/v1alpha1
kind: TrainJob
metadata:
  name: llm-finetune
spec:
  runtimeRef:
    name: torch-distributed
  trainer:
    image: my-training-image:latest
    command: ["torchrun"]
    args: ["train.py", "--epochs", "3"]
    numNodes: 4
    resourcesPerNode:
      requests:
        cpu: 2500m
        memory: 16Gi
      limits:
        nvidia.com/gpu: 1
"""


def test_full_manifest_maps_resources_and_scaling():
    """Full manifest maps resources and scaling."""
    result = trainjob.convert_text(_FULL_MANIFEST)
    text = result.scaffold
    # Head and workers are sized identically: TrainJob nodes are homogeneous.
    assert "head_cpu=3," in text  # 2500m rounds up to whole CPUs
    assert "head_memory='16Gi'," in text
    assert "head_gpu=1," in text
    assert "worker_cpu=3," in text
    assert "worker_memory='16Gi'," in text
    assert "worker_gpu=1," in text
    assert "worker_instances=4," in text
    assert "num_workers=4," in text
    assert "use_gpu=True," in text
    assert "name='llm-finetune'" in text
    assert "generated from TrainJob 'llm-finetune'" in text
    # Entrypoint surfaced as TODO comments, with args appended to command.
    assert "image:   my-training-image:latest" in text
    assert "command: torchrun train.py --epochs 3" in text
    assert result.warnings == []


def test_defaults_without_trainer_block():
    """Defaults without trainer block."""
    result = trainjob.convert(
        {
            "apiVersion": "trainer.kubeflow.org/v1alpha1",
            "kind": "TrainJob",
            "spec": {"runtimeRef": {"name": "torch-distributed"}},
        }
    )
    text = result.scaffold
    assert "name='imported-train-job'" in text
    assert "worker_instances=1," in text
    assert "num_workers=1," in text
    assert "use_gpu=False," in text
    assert "# TODO: size the cluster for your workload." in text
    assert any("no spec.trainer block" in w for w in result.warnings)


def test_non_torch_runtime_warns():
    """Non torch runtime warns."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    manifest["spec"]["runtimeRef"]["name"] = "deepspeed-distributed"
    result = trainjob.convert(manifest)
    assert any("not a torch runtime" in w for w in result.warnings)


def test_missing_runtime_ref_warns():
    """Missing runtime ref warns."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    del manifest["spec"]["runtimeRef"]
    result = trainjob.convert(manifest)
    assert any("names no runtime" in w for w in result.warnings)


def test_unmapped_spec_fields_warn():
    """Unmapped spec fields warn."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    manifest["spec"]["datasetConfig"] = {"storageUri": "s3://data"}
    manifest["spec"]["podSpecOverrides"] = []
    manifest["spec"]["suspend"] = False
    result = trainjob.convert(manifest)
    for field in ("datasetConfig", "podSpecOverrides", "suspend"):
        assert any(
            f"spec.{field} has no pipeline equivalent" in w for w in result.warnings
        )


def test_unmapped_trainer_fields_warn():
    """Unmapped trainer fields warn."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    manifest["spec"]["trainer"]["env"] = [{"name": "X", "value": "1"}]
    manifest["spec"]["trainer"]["numProcPerNode"] = "auto"
    result = trainjob.convert(manifest)
    assert any("spec.trainer.env has no pipeline" in w for w in result.warnings)
    assert any(
        "spec.trainer.numProcPerNode has no pipeline" in w for w in result.warnings
    )


def test_limits_fallback_for_cpu_and_memory():
    """Limits fallback for cpu and memory."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    manifest["spec"]["trainer"]["resourcesPerNode"] = {
        "limits": {"cpu": "8", "memory": "32Gi"}
    }
    result = trainjob.convert(manifest)
    assert "worker_cpu=8," in result.scaffold
    assert "worker_memory='32Gi'," in result.scaffold
    assert "use_gpu=False," in result.scaffold


def test_wrong_kind_raises():
    """Wrong kind raises."""
    with pytest.raises(trainjob.ManifestError, match="unsupported kind 'TFJob'"):
        trainjob.convert({"kind": "TFJob"})


def test_non_trainer_api_group_warns():
    """Non trainer api group warns."""
    manifest = yaml.safe_load(_FULL_MANIFEST)
    manifest["apiVersion"] = "example.com/v1"
    result = trainjob.convert(manifest)
    assert any("is not from trainer.kubeflow.org" in w for w in result.warnings)


def test_invalid_yaml_raises():
    """Invalid yaml raises."""
    with pytest.raises(trainjob.ManifestError, match="not valid YAML"):
        trainjob.convert_text("kind: [broken")


def test_non_mapping_input_raises():
    """Non mapping input raises."""
    with pytest.raises(trainjob.ManifestError, match="expected a YAML mapping"):
        trainjob.convert_text("- a list")


def test_scaffold_is_valid_python():
    """Scaffold is valid python."""
    result = trainjob.convert_text(_FULL_MANIFEST)
    compile(result.scaffold, "<scaffold>", "exec")
