"""Resolve a workflow's Pipeline CR from inline metadata and/or a pipeline YAML.

Run ``python -m michelangelo.uniflow.registration.pipeline_spec --help`` for
the command line used by registration tooling.

Precedence:

* Inline metadata (``@uniflow.workflow(name=..., ...)``) wins over a pipeline
  YAML (``--config-file``, or a sibling ``pipeline.yaml`` next to the workflow
  module). Every conflicting field produces a warning.
* Fields set only in the YAML fill the gaps. ``labels``, ``annotations`` and
  ``triggers`` merge key by key; ``notifications`` is replaced as a whole.
* ``type`` and ``spec.commit`` default (TRAIN, current git) only when inline
  metadata exists, so a YAML-only pipeline renders exactly as written.
* ``spec.manifest.filePath`` is always derived from the workflow's module.
"""

import argparse
import ast
import contextlib
import copy
import dataclasses
import importlib
import inspect
import json
import os
import sys
import traceback
from collections.abc import Sequence
from types import ModuleType
from typing import Any, Callable, Optional, Protocol, TextIO

import yaml

from michelangelo.uniflow.core.decorator import is_workflow
from michelangelo.uniflow.core.pipeline_metadata import (
    DEFAULT_PIPELINE_TYPE,
    LIST_FIELDS,
    MAP_FIELDS,
    UNIFLOW_IMAGE_ANNOTATION,
    PipelineMetadata,
    PipelineMetadataError,
    describe_function,
    get_pipeline_metadata,
    normalize_pipeline_type,
)
from michelangelo.uniflow.registration.git_info import (
    GitInfoProvider,
    SubprocessGitInfoProvider,
)

SCHEMA_VERSION = 1
DEFAULT_SIBLING_YAML = "pipeline.yaml"
DECORATOR_LABEL = "@uniflow.workflow"
MANIFEST_TYPE_UNIFLOW = "PIPELINE_MANIFEST_TYPE_UNIFLOW"
REQUIRED_FIELDS = ("name", "namespace")
METADATA_FIELDS = tuple(f.name for f in dataclasses.fields(PipelineMetadata))

# Metadata field -> location in the Pipeline CR, in the CR's canonical key order.
# MAP_FIELDS merge into the mapping at their path, key by key.
CR_FIELD_PATHS: dict[str, tuple[str, ...]] = {
    "name": ("metadata", "name"),
    "namespace": ("metadata", "namespace"),
    "labels": ("metadata", "labels"),
    "annotations": ("metadata", "annotations"),
    "image": ("metadata", "annotations", UNIFLOW_IMAGE_ANNOTATION),
    "description": ("spec", "description"),
    "owner": ("spec", "owner", "name"),
    "type": ("spec", "type"),
    "triggers": ("spec", "manifest", "triggerMap"),
    "git_ref": ("spec", "commit", "gitRef"),
    "branch": ("spec", "commit", "branch"),
    "notifications": ("spec", "notifications"),
}
_COMMIT_FIELDS = ("git_ref", "branch")
# Fields rendered after spec.manifest's own keys (type, filePath).
_AFTER_MANIFEST_FIELDS = ("triggers", *_COMMIT_FIELDS, "notifications")

# Pipeline YAML keys that registration fills in itself, so no value is lost
# when a YAML is replaced by inline metadata.
_DERIVED_YAML_KEYS = frozenset(
    {
        ("apiVersion",),
        ("kind",),
        ("spec", "manifest", "type"),
        ("spec", "manifest", "filePath"),
        ("spec", "manifest", "path"),
        ("spec", "manifest", "uniflowFunction"),
        ("spec", "manifest", "uniflowTar"),
        ("spec", "manifest", "content"),
    }
)


class WorkflowImportError(Exception):
    """Raised when the workflow module cannot be imported or inspected."""


class FileSystem(Protocol):
    """Minimal read-only filesystem used by the resolver."""

    def exists(self, path: str) -> bool:
        """Return whether ``path`` is an existing file."""

    def read_text(self, path: str) -> str:
        """Return the text content of ``path``."""


class LocalFileSystem:
    """``FileSystem`` backed by the local disk."""

    def exists(self, path: str) -> bool:
        """Return whether ``path`` is an existing file."""
        return os.path.isfile(path)

    def read_text(self, path: str) -> str:
        """Return the text content of ``path``."""
        with open(path, encoding="utf-8") as f:
            return f.read()


@dataclasses.dataclass(frozen=True)
class PipelineYaml:
    """A parsed pipeline YAML (a Pipeline CR such as ``pipeline.yaml``).

    Attributes:
        path: Where the YAML was read from.
        raw: The YAML document.
        metadata: Identity fields extracted from ``raw``.
        module: ``spec.manifest.filePath`` (or ``path``), if set.
        function: ``spec.manifest.uniflowFunction``, if set.
    """

    path: str
    raw: dict[str, Any]
    metadata: PipelineMetadata
    module: Optional[str] = None
    function: Optional[str] = None


@dataclasses.dataclass(frozen=True)
class ResolvedPipeline:
    """The resolved Pipeline CR plus everything a CLI needs to publish it.

    Attributes:
        metadata: Merged, defaulted pipeline metadata.
        module: Dotted module path of the workflow.
        function: Name of the workflow function.
        source_path: Workflow source file, relative to the pipeline root.
        source: Where the metadata came from: decorator, yaml or decorator+yaml.
        pipeline_cr: The rendered Pipeline CR.
        warnings: Non-fatal problems to show the user.
    """

    metadata: PipelineMetadata
    module: str
    function: str
    source_path: str
    source: str
    pipeline_cr: dict[str, Any]
    warnings: list[str]

    def to_dict(self) -> dict[str, Any]:
        """Return the stable JSON contract consumed by CLIs (schema v1)."""
        meta = self.metadata
        return {
            "schema_version": SCHEMA_VERSION,
            "name": meta.name,
            "namespace": meta.namespace,
            "description": meta.description,
            "owner": meta.owner,
            "type": meta.type,
            "image": meta.image,
            "commit": {"git_ref": meta.git_ref, "branch": meta.branch},
            "labels": meta.labels,
            "annotations": meta.annotations,
            "triggers": meta.triggers,
            "notifications": meta.notifications,
            "module": self.module,
            "function": self.function,
            "source_path": self.source_path,
            "source": self.source,
            "pipeline_cr": self.pipeline_cr,
            "warnings": list(self.warnings),
        }


def find_workflow_function(
    module: ModuleType, function: Optional[str] = None
) -> Callable[..., Any]:
    """Return the workflow function defined in ``module``.

    Args:
        module: The imported workflow module.
        function: Explicit function name. When omitted, the single workflow
            defined in the module is used; with several, the ``ctx.run(...)``
            target in the module disambiguates.

    Returns:
        The workflow function (as exposed by the module, i.e. wrapped).

    Raises:
        PipelineMetadataError: If no unambiguous workflow function is found.
    """
    if function:
        fn = getattr(module, function, None)
        if fn is None or not callable(fn):
            raise PipelineMetadataError(
                f"function {function!r} not found in module {module.__name__}"
            )
        if not is_workflow(inspect.unwrap(fn)):
            raise PipelineMetadataError(
                f"{module.__name__}.{function} is not a @uniflow.workflow() function"
            )
        return fn

    candidates = {
        attr: obj
        for attr, obj in vars(module).items()
        if callable(obj)
        and is_workflow(inspect.unwrap(obj))
        and getattr(inspect.unwrap(obj), "__module__", None) == module.__name__
    }
    if len(candidates) == 1:
        return next(iter(candidates.values()))

    for target in ctx_run_targets(_module_source(module)):
        if target in candidates:
            return candidates[target]

    if not candidates:
        raise PipelineMetadataError(
            f"no @uniflow.workflow() function found in module {module.__name__}"
        )
    raise PipelineMetadataError(
        f"multiple workflow functions in module {module.__name__}: "
        f"{', '.join(sorted(candidates))}; pass --function"
    )


def ctx_run_targets(source: Optional[str]) -> list[str]:
    """Return the names passed as first argument to ``ctx.run(...)`` calls."""
    if not source:
        return []
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return []
    return [
        node.args[0].id
        for node in ast.walk(tree)
        if isinstance(node, ast.Call)
        and isinstance(node.func, ast.Attribute)
        and node.func.attr == "run"
        and isinstance(node.func.value, ast.Name)
        and node.func.value.id == "ctx"
        and node.args
        and isinstance(node.args[0], ast.Name)
    ]


def parse_pipeline_yaml(path: str, raw: Any) -> PipelineYaml:
    """Extract the identity fields from a pipeline YAML document.

    Args:
        path: Where the YAML was read from (for messages).
        raw: The parsed YAML document.

    Returns:
        The parsed YAML.

    Raises:
        PipelineMetadataError: If the document is not a mapping.
    """
    if not isinstance(raw, dict):
        raise PipelineMetadataError(f"{path}: expected a Pipeline CR mapping")
    values: dict[str, Any] = {}
    for field, cr_path in CR_FIELD_PATHS.items():
        value = _get_path(raw, cr_path)
        if value is None:
            continue
        if field in MAP_FIELDS:
            if isinstance(value, dict):
                if field == "annotations":  # the image has its own field
                    value = {
                        k: v for k, v in value.items() if k != UNIFLOW_IMAGE_ANNOTATION
                    }
                values[field] = copy.deepcopy(value) or None
        elif field in LIST_FIELDS:
            if isinstance(value, list):
                values[field] = copy.deepcopy(value)
        else:
            values[field] = str(value)
    manifest = _get_path(raw, ("spec", "manifest")) or {}
    return PipelineYaml(
        path=path,
        raw=raw,
        metadata=PipelineMetadata(**values),
        module=manifest.get("filePath") or manifest.get("path"),
        function=manifest.get("uniflowFunction"),
    )


def load_pipeline_yaml(path: str, fs: FileSystem) -> PipelineYaml:
    """Read and parse a pipeline YAML."""
    try:
        raw = yaml.safe_load(fs.read_text(path))
    except (OSError, yaml.YAMLError) as e:
        raise PipelineMetadataError(f"cannot read {path}: {e}") from e
    return parse_pipeline_yaml(path, raw)


def load_sibling_yaml(
    module_file: Optional[str], fs: FileSystem, name: str = DEFAULT_SIBLING_YAML
) -> Optional[PipelineYaml]:
    """Return the pipeline YAML called ``name`` next to ``module_file``, if any."""
    if not module_file:
        return None
    path = os.path.join(os.path.dirname(module_file), name)
    if not fs.exists(path):
        return None
    return load_pipeline_yaml(path, fs)


def merge(
    inline: Optional[PipelineMetadata],
    pipeline_yaml: Optional[PipelineYaml],
    label: str = DECORATOR_LABEL,
) -> tuple[PipelineMetadata, list[str]]:
    """Merge inline metadata over pipeline YAML metadata.

    Args:
        inline: Metadata declared on the workflow, if any.
        pipeline_yaml: The pipeline YAML, if any.
        label: How the inline source is named in warnings.

    Returns:
        The merged metadata, plus one warning per conflicting field and
        warnings on what is left to migrate out of the YAML.
    """
    if pipeline_yaml is None:
        return inline or PipelineMetadata(), []
    if inline is None:
        return pipeline_yaml.metadata, []

    warnings = []
    merged: dict[str, Any] = {}
    for field, cr_path in CR_FIELD_PATHS.items():
        ours = getattr(inline, field)
        theirs = getattr(pipeline_yaml.metadata, field)
        if field in MAP_FIELDS and ours is not None and theirs is not None:
            for key, value in ours.items():
                if key in theirs and theirs[key] != value:
                    warnings.append(
                        _override_warning(
                            f'{label}({field}["{key}"]',
                            value,
                            ".".join((*cr_path, key)),
                            theirs[key],
                            pipeline_yaml.path,
                        )
                    )
            merged[field] = {**theirs, **ours}
            continue
        if ours is not None and theirs is not None and not _same(field, ours, theirs):
            warnings.append(
                _override_warning(
                    f"{label}({field}",
                    ours,
                    ".".join(cr_path),
                    theirs,
                    pipeline_yaml.path,
                )
            )
        merged[field] = ours if ours is not None else theirs
    warnings.extend(_migration_warnings(inline, pipeline_yaml, label))
    return PipelineMetadata(**merged), warnings


def apply_defaults(
    metadata: PipelineMetadata, git: GitInfoProvider, root: str
) -> PipelineMetadata:
    """Fill ``type`` (TRAIN) and ``git_ref``/``branch`` (current git) if unset."""
    updates: dict[str, Optional[str]] = {}
    if metadata.type is None:
        updates["type"] = DEFAULT_PIPELINE_TYPE
    if metadata.git_ref is None or metadata.branch is None:
        info = git.get(root)
        if metadata.git_ref is None:
            updates["git_ref"] = info.git_ref
        if metadata.branch is None:
            updates["branch"] = info.branch
    return dataclasses.replace(metadata, **updates)


def check_required(
    metadata: PipelineMetadata,
    where: str,
    required: Sequence[str] = REQUIRED_FIELDS,
    label: str = DECORATOR_LABEL,
) -> None:
    """Raise if any required field is missing.

    Args:
        metadata: The merged metadata.
        where: The workflow, as named in the error.
        required: Fields that must be set.
        label: How the inline source is named in the error.

    Raises:
        PipelineMetadataError: Naming the missing fields and where to add them.
    """
    missing = [f for f in required if not getattr(metadata, f)]
    if missing:
        raise PipelineMetadataError(
            f"{where}: missing required pipeline metadata: {', '.join(missing)}; "
            f"add {', '.join(f'{f}=...' for f in missing)} to {label}(...)"
        )


def build_pipeline_cr(
    metadata: PipelineMetadata,
    module: str,
    base: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    """Render the Pipeline CR.

    Args:
        metadata: Merged metadata; set fields overwrite ``base``.
        module: Dotted workflow module, written to ``spec.manifest.filePath``.
        base: A pipeline YAML document whose other keys are preserved.

    Returns:
        The Pipeline CR as a dict, in the canonical key order.
    """
    cr: dict[str, Any] = {"apiVersion": "michelangelo.api/v2", "kind": "Pipeline"}
    if base:
        cr.update(copy.deepcopy(base))
    for field in CR_FIELD_PATHS:
        if field not in _AFTER_MANIFEST_FIELDS:
            _put_field(cr, field, getattr(metadata, field))
    # spec.manifest sits between spec.type and spec.commit, as in a pipeline YAML.
    manifest = _ensure_path(cr, ("spec", "manifest"))
    manifest.setdefault("type", MANIFEST_TYPE_UNIFLOW)
    manifest["filePath"] = module
    manifest.pop("path", None)
    for field in _AFTER_MANIFEST_FIELDS:
        _put_field(cr, field, getattr(metadata, field))
    return cr


def resolve(
    *,
    module: Optional[str] = None,
    config_file: Optional[str] = None,
    function: Optional[str] = None,
    root: str = ".",
    sibling_yaml: Optional[str] = DEFAULT_SIBLING_YAML,
    required: Sequence[str] = REQUIRED_FIELDS,
    git: Optional[GitInfoProvider] = None,
    fs: Optional[FileSystem] = None,
    importer: Callable[[str], ModuleType] = importlib.import_module,
) -> ResolvedPipeline:
    """Resolve the Pipeline CR for a workflow.

    Exactly one of ``module`` (the YAML-free mode) or ``config_file`` (a
    pipeline YAML such as ``pipeline.yaml``) must be given.

    Args:
        module: Dotted workflow module, e.g. ``workflows.my_wf.workflow``.
        config_file: Path to a pipeline YAML.
        function: Workflow function name; discovered when omitted.
        root: Pipeline root; put on ``sys.path`` and used for git lookups.
        sibling_yaml: In ``module`` mode, the name of a pipeline YAML next to
            the workflow module to merge in; ``None`` disables the lookup.
        required: Fields that must be set once metadata is merged.
        git: Git information provider. Defaults to the ``git`` CLI.
        fs: Filesystem. Defaults to the local disk.
        importer: Module importer; injectable for tests.

    Returns:
        The resolved pipeline.

    Raises:
        PipelineMetadataError: If metadata is missing, invalid or ambiguous.
        WorkflowImportError: If the workflow module cannot be imported.
    """
    if (module is None) == (config_file is None):
        raise ValueError("exactly one of module or config_file is required")
    git = git or SubprocessGitInfoProvider()
    fs = fs or LocalFileSystem()
    root = os.path.abspath(root)

    config = None
    if config_file is not None:
        config = load_pipeline_yaml(config_file, fs)
        if not config.module:
            raise PipelineMetadataError(
                f"{config_file}: spec.manifest.filePath is required"
            )
        module, _, explicit = config.module.partition(":")
        function = function or explicit or config.function

    assert module is not None
    mod = _import_module(module, root, importer)
    fn = find_workflow_function(mod, function)
    pipeline_yaml = config
    if pipeline_yaml is None and sibling_yaml:
        module_file = getattr(mod, "__file__", None)
        pipeline_yaml = load_sibling_yaml(module_file, fs, sibling_yaml)
    inline = get_pipeline_metadata(fn)
    where = describe_function(fn)

    if inline is None and pipeline_yaml is None:
        raise PipelineMetadataError(
            f"{where}: no pipeline metadata; add "
            f"{', '.join(f'{f}=...' for f in required)} to {DECORATOR_LABEL}(...)"
        )

    metadata, warnings = merge(inline, pipeline_yaml)
    if inline is not None:
        metadata = apply_defaults(metadata, git, root)
        cr = build_pipeline_cr(
            metadata, module, base=pipeline_yaml.raw if pipeline_yaml else None
        )
        source = "decorator+yaml" if pipeline_yaml else "decorator"
    else:
        assert pipeline_yaml is not None
        cr = copy.deepcopy(pipeline_yaml.raw)
        source = "yaml"
    check_required(metadata, where, required)

    return ResolvedPipeline(
        metadata=metadata,
        module=module,
        function=inspect.unwrap(fn).__name__,
        source_path=module_to_source_path(module),
        source=source,
        pipeline_cr=cr,
        warnings=warnings,
    )


def module_to_source_path(module: str) -> str:
    """Return ``a/b/c.py`` for module ``a.b.c`` (relative to the pipeline root)."""
    return "/".join(module.split(".")) + ".py"


def _import_module(
    module: str, root: str, importer: Callable[[str], ModuleType]
) -> ModuleType:
    if root not in sys.path:
        sys.path.insert(0, root)
    try:
        return importer(module)
    except PipelineMetadataError:
        raise
    except Exception as e:
        raise WorkflowImportError(
            f"cannot import workflow module {module}: {type(e).__name__}: {e}"
        ) from e


def _module_source(module: ModuleType) -> Optional[str]:
    try:
        return inspect.getsource(module)
    except (OSError, TypeError):
        return None


def _same(field: str, ours: str, theirs: str) -> bool:
    if field == "type":
        try:
            return normalize_pipeline_type(ours) == normalize_pipeline_type(theirs)
        except PipelineMetadataError:
            return False
    return ours == theirs


def _migration_warnings(
    inline: PipelineMetadata, pipeline_yaml: PipelineYaml, label: str
) -> list[str]:
    """Return what is left to migrate from ``pipeline_yaml`` to ``inline``."""
    path = pipeline_yaml.path
    pending = []
    for field in CR_FIELD_PATHS:
        if field in _COMMIT_FIELDS:  # without it, the commit defaults to git
            continue
        ours = getattr(inline, field)
        theirs = getattr(pipeline_yaml.metadata, field)
        if theirs is None:
            continue
        if field in MAP_FIELDS:
            keys = [key for key in theirs if key not in (ours or {})]
            if keys:
                pending.append(f"{field} ({', '.join(keys)})")
        elif ours is None:
            pending.append(field)
    extra = [
        ".".join(key) for key in _leaf_paths(pipeline_yaml.raw) if not _covered(key)
    ]
    if not pending and not extra:
        return [f"{path} is redundant with {label}(...); delete it"]
    warnings = []
    if pending:
        warnings.append(
            f"{path} still supplies {', '.join(pending)}; move them into {label}(...)"
        )
    if extra:
        warnings.append(
            f"{path} also sets {', '.join(extra)}, which {label}(...) cannot "
            f"express; keep {path} for them"
        )
    return warnings


def _covered(key: tuple[str, ...]) -> bool:
    """Whether inline metadata or registration can set YAML key ``key``."""
    if key in _DERIVED_YAML_KEYS:
        return True
    return any(key[: len(path)] == path for path in CR_FIELD_PATHS.values())


def _override_warning(
    ours_label: str, ours: Any, theirs_path: str, theirs: Any, yaml_path: str
) -> str:
    """Return ``label(field="a") overrides path="b" from yaml``."""
    if isinstance(ours, str) and isinstance(theirs, str):
        return (
            f'{ours_label}="{ours}") overrides {theirs_path}="{theirs}" '
            f"from {yaml_path}"
        )
    return f"{ours_label}) overrides {theirs_path} from {yaml_path}"


def _leaf_paths(doc: Any, prefix: tuple[str, ...] = ()) -> list[tuple[str, ...]]:
    if not isinstance(doc, dict):
        return [prefix] if prefix else []
    paths = []
    for key, value in doc.items():
        paths.extend(_leaf_paths(value, (*prefix, str(key))))
    return paths


def _get_path(doc: Any, path: Sequence[str]) -> Any:
    for key in path:
        if not isinstance(doc, dict):
            return None
        doc = doc.get(key)
    return doc


def _ensure_path(doc: dict[str, Any], path: Sequence[str]) -> dict[str, Any]:
    for key in path:
        child = doc.get(key)
        if not isinstance(child, dict):
            child = {}
            doc[key] = child
        doc = child
    return doc


def _put_field(cr: dict[str, Any], field: str, value: Any) -> None:
    """Write metadata ``field`` into ``cr``; mappings merge key by key."""
    if value is None:
        return
    path = CR_FIELD_PATHS[field]
    if field in MAP_FIELDS:
        _ensure_path(cr, path).update(copy.deepcopy(value))
    else:
        _ensure_path(cr, path[:-1])[path[-1]] = copy.deepcopy(value)


# Command line. stdout carries only the result (the JSON contract, or the bare
# Pipeline CR as YAML); warnings and errors go to stderr.
EXIT_OK = 0
EXIT_INTERNAL = 1
EXIT_USAGE = 2
EXIT_INVALID = 3

PROG = "python -m michelangelo.uniflow.registration.pipeline_spec"


def build_parser() -> argparse.ArgumentParser:
    """Return the command line argument parser."""
    parser = argparse.ArgumentParser(
        prog=PROG,
        description=(
            "Resolve a Uniflow workflow's Pipeline CR from "
            "@uniflow.workflow(name=..., ...) metadata and/or a pipeline YAML. "
            "Exit codes: 0 ok, 2 usage error, 3 invalid metadata or workflow "
            "import error, 1 internal error."
        ),
    )
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument(
        "--module", help="dotted workflow module, e.g. workflows.my_wf.workflow"
    )
    source.add_argument("--config-file", help="path to a pipeline YAML")
    parser.add_argument("--function", help="workflow function name (auto-detected)")
    parser.add_argument(
        "--root", default=".", help="pipeline root, added to sys.path (default: .)"
    )
    parser.add_argument(
        "--sibling-yaml",
        default=DEFAULT_SIBLING_YAML,
        metavar="NAME",
        help=(
            "with --module, the pipeline YAML next to the workflow module to merge "
            f"in (default: {DEFAULT_SIBLING_YAML}; '' disables the lookup)"
        ),
    )
    parser.add_argument(
        "--require",
        action="append",
        default=[],
        choices=METADATA_FIELDS,
        metavar="FIELD",
        help=(
            "also require FIELD (repeatable; always required: "
            f"{', '.join(REQUIRED_FIELDS)}; choices: {', '.join(METADATA_FIELDS)})"
        ),
    )
    parser.add_argument(
        "--format",
        choices=("json", "yaml"),
        default="json",
        help="json: full resolution contract; yaml: the Pipeline CR only",
    )
    return parser


def main(
    argv: Optional[list[str]] = None,
    stdout: Optional[TextIO] = None,
    stderr: Optional[TextIO] = None,
    resolver: Callable[..., ResolvedPipeline] = resolve,
) -> int:
    """Run the command line and return its exit code."""
    stdout = stdout or sys.stdout
    stderr = stderr or sys.stderr
    try:
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            args = build_parser().parse_args(argv)
    except SystemExit as e:
        return EXIT_OK if e.code == 0 else EXIT_USAGE

    try:
        # Importing user workflow code may print; keep stdout machine-readable.
        with contextlib.redirect_stdout(stderr):
            resolved = resolver(
                module=args.module,
                config_file=args.config_file,
                function=args.function,
                root=args.root,
                sibling_yaml=args.sibling_yaml or None,
                required=tuple(dict.fromkeys((*REQUIRED_FIELDS, *args.require))),
            )
    except (PipelineMetadataError, WorkflowImportError) as e:
        print(f"error: {e}", file=stderr)
        return EXIT_INVALID
    except Exception:
        traceback.print_exc(file=stderr)
        return EXIT_INTERNAL

    for warning in resolved.warnings:
        print(f"warning: {warning}", file=stderr)
    if args.format == "yaml":
        stdout.write(yaml.safe_dump(resolved.pipeline_cr, sort_keys=False))
    else:
        stdout.write(json.dumps(resolved.to_dict(), indent=2) + "\n")
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
