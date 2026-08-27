"""Typed configuration dataclasses for the tabular assembler task.

Each dataclass configures one framework-specific assembler path (custom
Python-backend or PyTorch/Lightning) or, for ``TabularAssemblerConfig``,
selects between them. Plain dataclasses (rather than a pydantic/ORM model)
keep validation, serialisation, and inspection by the workflow engine simple
and dependency-free at pipeline-definition time.

Consumers may subclass these to add provider-specific fields (e.g. a custom
storage toggle) without modifying this module.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class CustomAssemblerConfig:
    """Configuration for the custom (Python-backend) assembler path.

    Attributes:
        custom_batch_processing: When ``True``, the model implementation
            handles batching itself and receives inputs with an extra leading
            batch dimension on top of the model schema (schema shape
            ``[n, ..., m]`` becomes ``[batch, n, ..., m]``). When
            ``None``/``False``, Triton batches automatically and the model
            sees schema-shaped inputs.
        additional_import_prefixes: Extra Python module prefixes whose source
            files are bundled into the package. Useful when the model class
            uses dynamic imports (e.g. ``importlib``) that static analysis
            misses. Each prefix is resolved recursively.
        include_import_prefixes: Module prefixes the packager's static import
            analysis is restricted to when deciding which of the model
            class's imported modules to bundle. When ``None`` (the default),
            every reachable imported module is considered, which is
            unbounded and can be extremely slow in environments with a very
            large importable module graph (e.g. a monorepo where the model
            class's own package pulls in thousands of transitively-importable
            modules) — set this to scope the walk to the module prefixes that
            actually matter for the model (e.g. ``["mypkg.models"]`` if the
            model class and its real dependencies all live under that one
            package).
        archive_deployable_package: When ``True``, the deployable package is
            archived into a single tar file before upload (one storage
            object, e.g. ``.../deployable/model.tar``) instead of the
            default: uploaded as loose files under a directory prefix (e.g.
            ``.../deployable/model/1/model.pt``), preserving its internal
            structure. Enable this only if whatever consumes the deployable
            artifact expects a single downloadable archive.

    Example:
        >>> CustomAssemblerConfig(custom_batch_processing=True).custom_batch_processing
        True
    """

    custom_batch_processing: bool | None = None
    additional_import_prefixes: list[str] | None = None
    include_import_prefixes: list[str] | None = None
    archive_deployable_package: bool = False


@dataclass
class TorchAssemblerConfig:
    """Configuration for the PyTorch/Lightning assembler path.

    Attributes:
        backend: Triton backend used for the deployable package — one of
            ``"pytorch"``, ``"python"``, ``"onnxruntime"``.
            ``None`` selects the packager default (TorchScript/PyTorch).
            Validation of supported values is performed by the packager, not
            here, to keep a single source of truth.
        include_import_prefixes: Module prefixes the packager's static import
            analysis is restricted to when deciding which of the model
            class's imported modules to bundle. When ``None`` (the default),
            every reachable imported module is considered, which is
            unbounded and can be extremely slow (or reach unrelated,
            side-effecting modules) in environments with a very large
            importable module graph — set this to scope the walk to the
            module prefixes that actually matter for the model. Mirrors
            ``CustomAssemblerConfig.include_import_prefixes``.
        archive_deployable_package: When ``True``, the deployable package is
            archived into a single tar file before upload (one storage
            object, e.g. ``.../deployable/model.tar``) instead of the
            default: uploaded as loose files under a directory prefix (e.g.
            ``.../deployable/model/1/model.pt``), preserving its internal
            structure. Enable this only if whatever consumes the deployable
            artifact expects a single downloadable archive.

    Example:
        >>> TorchAssemblerConfig(backend="onnxruntime")
        TorchAssemblerConfig(backend='onnxruntime')
    """

    backend: str | None = None
    include_import_prefixes: list[str] | None = None
    archive_deployable_package: bool = False


@dataclass
class TabularAssemblerConfig:
    """Top-level configuration for the tabular assembler task.

    Selects and parameterises the framework-specific assembler path.
    ``custom`` and ``torch`` carry path-specific options; ``model_class``
    overrides the model class resolved from the trained model's metadata.

    Attributes:
        model_class: Fully-qualified model class (e.g.
            ``"mypkg.models.MyModel"``). When set, overrides the class
            recorded in the trained model's metadata and can force the
            custom path.
        custom: Options for the custom (Python-backend) path.
        torch: Options for the PyTorch/Lightning path.

    Example:
        >>> torch_cfg = TorchAssemblerConfig(backend="pytorch")
        >>> config = TabularAssemblerConfig(torch=torch_cfg)
        >>> config.torch.backend
        'pytorch'
    """

    model_class: str | None = None
    custom: CustomAssemblerConfig | None = None
    torch: TorchAssemblerConfig | None = None
