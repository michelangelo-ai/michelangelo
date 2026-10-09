"""Test helpers: temporary pipeline roots with importable workflow modules."""

import contextlib
import importlib
import os
import sys
import tempfile
import textwrap
from collections.abc import Iterator
from typing import Optional

from michelangelo.uniflow.registration.git_info import GitInfo


@contextlib.contextmanager
def pipeline_root(files: dict[str, str]) -> Iterator[str]:
    """Write ``files`` under a temporary pipeline root and yield its path.

    Missing ``__init__.py`` files are created so every directory is a package.
    The root is put on ``sys.path`` and every module imported from it is
    purged afterwards, so tests can reuse module names such as ``workflows``.
    """
    with tempfile.TemporaryDirectory() as root:
        top_level = set()
        for rel_path, content in files.items():
            path = os.path.join(root, rel_path)
            os.makedirs(os.path.dirname(path), exist_ok=True)
            with open(path, "w", encoding="utf-8") as f:
                f.write(textwrap.dedent(content))
            parts = rel_path.split("/")
            top_level.add(parts[0][:-3] if parts[0].endswith(".py") else parts[0])
            for depth in range(1, len(parts)):
                init = os.path.join(root, *parts[:depth], "__init__.py")
                if not os.path.exists(init):
                    open(init, "w").close()
        importlib.invalidate_caches()
        sys.path.insert(0, root)
        try:
            yield root
        finally:
            for name in list(sys.modules):
                if name.split(".")[0] in top_level:
                    del sys.modules[name]
            while root in sys.path:
                sys.path.remove(root)


class FakeGit:
    """``GitInfoProvider`` returning fixed values and counting calls."""

    def __init__(
        self, git_ref: Optional[str] = "master", branch: Optional[str] = "master"
    ):
        """Initialize with the values to return."""
        self.info = GitInfo(git_ref=git_ref, branch=branch)
        self.calls = 0

    def get(self, root: str) -> GitInfo:
        """Return the fixed git information."""
        self.calls += 1
        return self.info
