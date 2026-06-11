"""OS environment utilities for Uniflow workflows."""

import os as _os


class _Environ:
    """Dict-like access to OS environment variables for local workflow runs.

    This object is recognised by the Uniflow transpiler as a Starlark plugin
    bound to "os.environ".  Any attribute access on it in a workflow function
    (e.g. ``environ.get("KEY")``) is transpiled to ``__os__.environ.get("KEY")``
    in Starlark, where ``__os__.environ`` is the ``starlark.Dict`` injected by
    the starlark-worker ``os`` plugin.

    On local runs the methods delegate to the real ``os.environ`` dict so the
    same workflow code works without changes.

    Usage in a workflow::

        from michelangelo.uniflow.core.lib.os import environ

        @workflow()
        def my_workflow():
            last_ts = environ.get("LAST_SCHEDULE_TIMESTAMP")
            if last_ts != None:
                # incremental: process data since int(last_ts)
                ...
    """

    # Markers read by the Uniflow transpiler (build.py / FunctionTransformer).
    _uf_star_plugin = True
    _uf_star_plugin_binding = "os.environ"

    def get(self, key: str, default=None):
        """Return the value for *key* if present, else *default*.

        Parameters:
            key (str): Environment variable name.
            default: Value to return when *key* is absent. Defaults to None.

        Returns:
            str | None
        """
        return _os.environ.get(key, default)

    def __getitem__(self, key: str) -> str:
        """Return the value for *key*, raising KeyError when absent."""
        return _os.environ[key]

    def __contains__(self, key: str) -> bool:
        """Return True if *key* is set in the environment."""
        return key in _os.environ


environ = _Environ()
