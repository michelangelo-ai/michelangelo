r"""Assemble a declarative interpreter release.

The tool reads a release source file, generates the interpreter main.star,
packages it with the engine and the task type factories, and writes two
files:

- <out>/<version>.tar.gz: the interpreter tar. The operator uploads it to
  the release artifact.
- <out>/<version>.yaml: the release file that the apiserver loads. Its
  digest is the sha256 of the tar bytes.

The same source on the same Python and zlib gives the same bytes.

Usage::

    python -m michelangelo.uniflow.declarative.assemble \\
        michelangelo/uniflow/declarative/releases/core-v0.1.0.src.yaml \\
        --root .. --out ../go/worker/declarative/testdata/release
"""

import argparse
import hashlib
import json
import os
import re
import sys
import tempfile
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import Optional

import yaml

from michelangelo.uniflow.core.build import build_star

ENGINE = "python/michelangelo/uniflow/declarative/engine.star"
BUILD_DIR = ".declarative-build"

_TOP_KEYS = ("version", "artifact", "task_types")
_TYPE_KEYS = ("factory", "job_specs_key", "capabilities", "config", "job_specs")
_RESERVED_FIELDS = ("alias", "retry_attempts")
_CAPABILITIES = ("outcome_mode", "retry")
_FACTORY_ALIAS_PREFIX = "factory_"
_VERSION = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]*")
_ASCII_IDENTIFIER = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")


class ReleaseSourceError(ValueError):
    """A release source file breaks a rule of the assembly tool."""


@dataclass(frozen=True)
class TaskTypeSource:
    """One task type of a release source.

    Attributes:
        name: The task type name, such as "core/ray".
        factory_file: The factory .star file, relative to the repo root.
        factory_function: The factory function in factory_file.
        job_specs_key: The job_specs key that the engine reads for this type.
        capabilities: The capabilities of the type, such as "retry".
        config: The config schema, as the release file holds it.
        job_specs: The job_specs schema, as the release file holds it.
    """

    name: str
    factory_file: str
    factory_function: str
    job_specs_key: str
    capabilities: list
    config: dict
    job_specs: dict


@dataclass(frozen=True)
class ReleaseSource:
    """A parsed release source file."""

    version: str
    artifact: str
    task_types: dict


def _identifier(name: str) -> str:
    return name.replace("/", "_").replace(".", "_")


def _is_identifier(name: str) -> bool:
    return _ASCII_IDENTIFIER.fullmatch(name) is not None


def _check_keys(doc: dict, allowed: tuple, where: str) -> None:
    for key in doc:
        if key not in allowed:
            raise ReleaseSourceError(f"{where}: unknown key {json.dumps(key)}")


def _require(doc: dict, key: str, kind: type, where: str):
    value = doc.get(key)
    if not isinstance(value, kind) or not value:
        raise ReleaseSourceError(f"{where}: {key} must be a non-empty {kind.__name__}")
    return value


def _parse_type(name: str, doc) -> TaskTypeSource:
    where = f"task type {json.dumps(name)}"
    if not isinstance(doc, dict):
        raise ReleaseSourceError(f"{where}: must be a mapping")
    _check_keys(doc, _TYPE_KEYS, where)
    rule = 'is not an identifier after mapping "/" and "." to "_"'
    if not _is_identifier(_identifier(name)):
        raise ReleaseSourceError(f"{where}: the name {rule}")

    factory = _require(doc, "factory", str, where)
    factory_file, _, factory_function = factory.rpartition(":")
    if not factory_file.endswith(".star") or not _is_identifier(factory_function):
        raise ReleaseSourceError(
            f"{where}: factory {json.dumps(factory)} must be <file>.star:<function>"
        )

    key = _require(doc, "job_specs_key", str, where)
    if not _is_identifier(_identifier(key)):
        raise ReleaseSourceError(f"{where}: job_specs_key {json.dumps(key)} {rule}")

    capabilities = doc.get("capabilities")
    if capabilities is None:
        capabilities = []
    if not isinstance(capabilities, list) or not all(
        isinstance(c, str) for c in capabilities
    ):
        raise ReleaseSourceError(f"{where}: capabilities must be a list of strings")
    for capability in capabilities:
        if capability not in _CAPABILITIES:
            known = ", ".join(json.dumps(c) for c in _CAPABILITIES)
            raise ReleaseSourceError(
                f"{where}: unknown capability {json.dumps(capability)}; known: {known}"
            )
    config = doc.get("config") or {}
    job_specs = doc.get("job_specs") or {}
    for other in job_specs:
        if other != key:
            raise ReleaseSourceError(
                f"{where}: job_specs key {json.dumps(other)}"
                f" is not job_specs_key {json.dumps(key)}"
            )
    job_fields = job_specs.get(key) or {}
    for field in [*config, *job_fields]:
        if field in _RESERVED_FIELDS:
            raise ReleaseSourceError(
                f"{where}: field {json.dumps(field)} is reserved; the engine sets it"
            )
        if field in config and field in job_fields:
            raise ReleaseSourceError(
                f"{where}: field {json.dumps(field)} is in both config and job_specs"
            )
    return TaskTypeSource(
        name=name,
        factory_file=factory_file,
        factory_function=factory_function,
        job_specs_key=key,
        capabilities=list(capabilities),
        config=config,
        job_specs=job_specs,
    )


def parse_source(doc) -> ReleaseSource:
    """Parse and check a release source document.

    Args:
        doc: The release source, as YAML loads it.

    Returns:
        The parsed source.

    Raises:
        ReleaseSourceError: If the source breaks a rule. The message names
            the rule.
    """
    where = "release source"
    if not isinstance(doc, dict):
        raise ReleaseSourceError(f"{where}: must be a mapping")
    _check_keys(doc, _TOP_KEYS, where)
    version = _require(doc, "version", str, where)
    if not _VERSION.fullmatch(version):
        raise ReleaseSourceError(
            f"{where}: version {json.dumps(version)} must match {_VERSION.pattern}"
        )
    artifact = _require(doc, "artifact", str, where)
    types = _require(doc, "task_types", dict, where)
    parsed = {name: _parse_type(name, t) for name, t in types.items()}
    aliases = {}
    for name in parsed:
        other = aliases.setdefault(_identifier(name), name)
        if other != name:
            raise ReleaseSourceError(
                f"{where}: task types {json.dumps(other)} and {json.dumps(name)}"
                " map to the same identifier"
            )
    return ReleaseSource(version=version, artifact=artifact, task_types=parsed)


def generate_main(source: ReleaseSource, build_dir: Path, root: Path) -> str:
    """Return the main.star source of the release.

    main.star loads the engine and each factory by a path relative to
    build_dir, defines REGISTRY, and defines main(spec, params).

    Args:
        source: The parsed release source.
        build_dir: The directory that main.star is written to.
        root: The repo root that the source paths are relative to.

    Returns:
        The Starlark source.
    """

    def rel(path: str) -> str:
        return json.dumps(Path(os.path.relpath(root / path, build_dir)).as_posix())

    lines = [f'load({rel(ENGINE)}, "run")']
    entries = []
    for t in source.task_types.values():
        alias = _FACTORY_ALIAS_PREFIX + _identifier(t.name)
        lines.append(
            f"load({rel(t.factory_file)}, {alias} = {json.dumps(t.factory_function)})"
        )
        entries.append(
            f"    {json.dumps(t.name)}: {{"
            f'"factory": {alias}, '
            f'"job_specs_key": {json.dumps(t.job_specs_key)}, '
            f'"capabilities": {json.dumps(t.capabilities)}}},'
        )
    lines += ["", "REGISTRY = {", *entries, "}", ""]
    lines += ["def main(spec, params):", "    return run(spec, params, REGISTRY)", ""]
    return "\n".join(lines)


def release_document(source: ReleaseSource, digest: str) -> dict:
    """Return the release file that the apiserver loads.

    It has no factory or job_specs_key: those exist only in the tar.
    """
    return {
        "version": source.version,
        "artifact": source.artifact,
        "digest": digest,
        "task_types": {
            t.name: {
                "capabilities": t.capabilities,
                "config": t.config,
                "job_specs": t.job_specs,
            }
            for t in source.task_types.values()
        },
    }


def assemble(source_file: Path, root: Path, out_dir: Path) -> tuple:
    """Build the release tar and the release file of a release source.

    Args:
        source_file: The release source YAML file.
        root: The repo root that the source paths are relative to.
        out_dir: The directory for <version>.tar.gz and <version>.yaml.

    Returns:
        The paths of the tar and the release file.

    Raises:
        ReleaseSourceError: If the source breaks a rule.
        ValueError: If a loaded file is outside root.
        FileExistsError: If <version>.tar.gz exists with other bytes.
    """
    root = Path(root).resolve()
    source = parse_source(yaml.safe_load(Path(source_file).read_text()))

    build_dir = root / BUILD_DIR / source.version
    build_dir.mkdir(parents=True, exist_ok=True)
    main_file = build_dir / "main.star"
    main_file.write_text(generate_main(source, build_dir, root))

    buf = BytesIO()
    build_star(main_file, root).to_tarball(buf, gzip_mtime=0)
    tar_bytes = buf.getvalue()
    digest = "sha256:" + hashlib.sha256(tar_bytes).hexdigest()

    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    tar_path = out_dir / f"{source.version}.tar.gz"
    release_path = out_dir / f"{source.version}.yaml"
    if tar_path.exists() and tar_path.read_bytes() != tar_bytes:
        raise FileExistsError(
            f"{tar_path} exists with other bytes;"
            " bump the version in the release source"
        )

    header = (
        f"# Generated by michelangelo.uniflow.declarative.assemble from"
        f" {Path(source_file).name}. Do not edit.\n"
    )
    release = header + yaml.safe_dump(
        release_document(source, digest),
        sort_keys=False,
        default_flow_style=None,
    )
    _atomic_write(tar_path, tar_bytes)
    _atomic_write(release_path, release.encode())
    return tar_path, release_path


def _atomic_write(path: Path, data: bytes) -> None:
    if path.exists() and path.read_bytes() == data:
        return
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.")
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(data)
        os.chmod(tmp, 0o644)
        os.replace(tmp, path)
    except BaseException:
        os.unlink(tmp)
        raise


def main(argv: Optional[list] = None) -> None:
    """Run the assembly tool from the command line."""
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("source", type=Path, help="release source YAML file")
    p.add_argument("--root", type=Path, required=True, help="repo root")
    p.add_argument("--out", type=Path, required=True, help="output directory")
    a = p.parse_args(argv)
    try:
        tar_path, release_path = assemble(a.source, a.root, a.out)
    except (ReleaseSourceError, OSError) as e:
        print(f"error: {e}", file=sys.stderr)
        sys.exit(2)
    print(tar_path)
    print(release_path)


if __name__ == "__main__":
    main()
