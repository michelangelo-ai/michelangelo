"""Emit a one-time warning when running a nightly (dev) build."""

import warnings


def _check_nightly():
    try:
        from importlib.metadata import version

        ver = version("michelangelo")
    except Exception:
        return

    if ".dev" not in ver:
        return

    warnings.warn(
        f"You are using a nightly development build of Michelangelo ({ver}). "
        "This build is not supported for production use. "
        "Install a stable release: pip install michelangelo",
        UserWarning,
        stacklevel=2,
    )


_check_nightly()
