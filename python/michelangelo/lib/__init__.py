"""Core library package for Michelangelo.

Houses the model manager, artifact manager, trainer, and related subpackages.

Callers should import from the submodules directly (e.g.
``michelangelo.lib.model_manager.packager``); this ``__init__.py``
intentionally exports nothing.

Note: unlike the top-level :mod:`michelangelo` package, this subpackage does
not need PEP 420 / ``pkgutil``-style namespace merging — its contents are
fully authored in this repository — so it is a regular package (with this
``__init__.py``) rather than an implicit namespace package. Regular
subpackages resolve reliably under ``bazel``-generated wheel installs, where
implicit namespace subpackages were found not to be picked up correctly by
the runfiles tree built from ``requirement("michelangelo-ai")``.
"""
