"""Fixture submodule that raises AttributeError at import time.

Regression coverage for a real crash: a submodule's import-time side
effects can raise exceptions outside (ImportError, TypeError, SystemExit)
-- e.g. CPython's own `test.__main__` package, imported while walking a
real PyTorch model's dependency tree, raised AttributeError trying to
patch `torch.distributed.config.__file__` (torch's ConfigModule disallows
arbitrary attribute writes).
"""


class _Frozen:
    __slots__ = ()

    def __setattr__(self, name, value):
        raise AttributeError(f"{name} does not exist")


_Frozen().unknown_attr = "boom"
