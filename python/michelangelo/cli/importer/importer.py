"""``ma import``: convert job manifests from other systems into Michelangelo ones."""

import sys
from pathlib import Path

import yaml

from michelangelo.cli.importer import base, pytorchjob, rayjob, trainjob

short_description = "Convert job manifests from other systems."

description = """
Convert a Kubernetes job manifest into its Michelangelo equivalent. The
generated file is a starting point, not a finished artifact: what maps is
mapped for you, the rest is marked with TODOs and warnings. Kubeflow Trainer
PyTorchJobs and TrainJobs become Uniflow pipeline scaffolds; KubeRay RayJobs
become michelangelo.api/v2 RayJob (and, for inline cluster specs, RayCluster)
manifests.
"""

# kind -> converter entry point. Additional kinds plug in here.
_CONVERTERS = {
    "PyTorchJob": pytorchjob.convert_text,
    "RayJob": rayjob.convert_text,
    "TrainJob": trainjob.convert_text,
}


def init_arguments(parser):
    """Register ``ma import`` arguments on the given subparser."""
    parser.add_argument(
        "manifest",
        help="Path to the job manifest to convert (YAML)",
    )
    parser.add_argument(
        "-o",
        "--output",
        help="Write the generated file here instead of stdout",
    )


def run(ns) -> int:
    """Convert the manifest named by the parsed CLI arguments."""
    path = Path(ns.manifest)
    try:
        text = path.read_text()
    except OSError as exc:
        print(f"error: cannot read {path}: {exc}", file=sys.stderr)
        return 1

    kind = _detect_kind(text)
    converter = _CONVERTERS.get(kind)
    if converter is None:
        supported = ", ".join(sorted(_CONVERTERS))
        print(
            f"error: unsupported manifest kind {kind!r} (supported: {supported})",
            file=sys.stderr,
        )
        return 1

    try:
        result = converter(text)
    except base.ManifestError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    for warning in result.warnings:
        print(f"warning: {warning}", file=sys.stderr)

    if ns.output:
        Path(ns.output).write_text(result.scaffold)
        print(f"wrote {ns.output}", file=sys.stderr)
    else:
        print(result.scaffold, end="")
    return 0


def _detect_kind(text):
    """Best-effort read of the manifest's kind; converters re-validate."""
    try:
        manifest = yaml.safe_load(text)
    except yaml.YAMLError:
        return None
    if isinstance(manifest, dict):
        return manifest.get("kind")
    return None
