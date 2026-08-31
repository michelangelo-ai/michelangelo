"""Tests for the ``ma import`` CLI surface."""

import argparse

import pytest

from michelangelo.cli import cli
from michelangelo.cli.importer import importer

_MANIFEST = """
apiVersion: kubeflow.org/v1
kind: PyTorchJob
metadata:
  name: cli-test-job
spec:
  pytorchReplicaSpecs:
    Worker:
      replicas: 2
      template:
        spec:
          containers:
            - name: pytorch
              image: img:1
"""


def _parse(args):
    parser = argparse.ArgumentParser()
    importer.init_arguments(parser)
    return parser.parse_args(args)


def test_run_writes_scaffold_to_stdout(tmp_path, capsys):
    """Run writes scaffold to stdout."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(_MANIFEST)
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 0
    assert "worker_instances=2," in captured.out
    assert "name='cli-test-job'" in captured.out


def test_run_writes_scaffold_to_output_file(tmp_path, capsys):
    """Run writes scaffold to output file."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(_MANIFEST)
    out = tmp_path / "pipeline.py"
    rc = importer.run(_parse([str(manifest), "-o", str(out)]))
    captured = capsys.readouterr()
    assert rc == 0
    assert captured.out == ""
    assert f"wrote {out}" in captured.err
    assert "worker_instances=2," in out.read_text()


def test_run_reports_warnings_on_stderr(tmp_path, capsys):
    """Run reports warnings on stderr."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(_MANIFEST.replace("spec:\n", "spec:\n  nprocPerNode: '2'\n", 1))
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 0
    assert "warning: spec.nprocPerNode" in captured.err


def test_run_missing_file_fails(tmp_path, capsys):
    """Run missing file fails."""
    rc = importer.run(_parse([str(tmp_path / "nope.yaml")]))
    captured = capsys.readouterr()
    assert rc == 1
    assert "cannot read" in captured.err


def test_run_unsupported_kind_fails(tmp_path, capsys):
    """Run unsupported kind fails."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text("kind: TFJob\n")
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 1
    assert "unsupported manifest kind 'TFJob'" in captured.err
    assert "supported: PyTorchJob, RayJob, TrainJob" in captured.err


def test_run_invalid_manifest_fails(tmp_path, capsys):
    """Run invalid manifest fails."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text("kind: PyTorchJob\nspec: {}\n")
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 1
    assert "error: spec.pytorchReplicaSpecs" in captured.err


def test_run_dispatches_rayjob_kind(tmp_path, capsys):
    """Run dispatches rayjob kind."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(
        "apiVersion: ray.io/v1\n"
        "kind: RayJob\n"
        "metadata: {name: rj}\n"
        "spec:\n"
        "  entrypoint: python x.py\n"
        "  clusterSelector: {ray.io/cluster: c1}\n"
    )
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 0
    assert "kind: RayJob" in captured.out
    assert "michelangelo.api/v2" in captured.out
    assert "warning: KubeRay manifests carry no user identity" in captured.err


def test_run_dispatches_trainjob_kind(tmp_path, capsys):
    """Run dispatches trainjob kind."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(
        "apiVersion: trainer.kubeflow.org/v1alpha1\n"
        "kind: TrainJob\n"
        "metadata: {name: tj}\n"
        "spec:\n"
        "  runtimeRef: {name: torch-distributed}\n"
        "  trainer: {numNodes: 2}\n"
    )
    rc = importer.run(_parse([str(manifest)]))
    captured = capsys.readouterr()
    assert rc == 0
    assert "generated from TrainJob 'tj'" in captured.out
    assert "num_workers=2," in captured.out


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        ("kind: PyTorchJob", "PyTorchJob"),
        ("kind: [broken", None),
        ("- a list", None),
        ("{}", None),
    ],
)
def test_detect_kind(text, expected):
    """Detect kind."""
    assert importer._detect_kind(text) == expected


def test_cli_dispatches_import_entity(tmp_path, capsys):
    """Cli dispatches import entity."""
    manifest = tmp_path / "job.yaml"
    manifest.write_text(_MANIFEST)
    rc = cli.main(["import", str(manifest)])
    captured = capsys.readouterr()
    assert rc == 0
    assert "worker_instances=2," in captured.out
