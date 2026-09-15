"""``ma import``: convert training-job manifests into Uniflow pipeline scaffolds."""

import sys
from pathlib import Path

import yaml

from michelangelo.cli.importer import pytorchjob

short_description = "Convert training-job manifests into pipeline scaffolds."

description = """
Convert a Kubernetes training-job manifest into a Michelangelo pipeline
scaffold. The generated file is a starting point, not a finished pipeline:
cluster sizing is mapped for you, the training code itself is marked with
TODOs. Currently supports Kubeflow Trainer PyTorchJob manifests.
"""

# kind -> converter entry point. Additional kinds plug in here.
_CONVERTERS = {
    "PyTorchJob": pytorchjob.convert_text,
}


def init_arguments(parser):
    """Register ``ma import`` arguments on the given subparser."""
    parser.add_argument(
        "manifest",
        help="Path to the training-job manifest to convert (YAML)",
    )
    parser.add_argument(
        "-o",
        "--output",
        help="Write the generated pipeline to this file instead of stdout",
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
    except pytorchjob.ManifestError as exc:
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
