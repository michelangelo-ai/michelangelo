"""Regression test for the ``michelangelo.uniflow`` PEP 561 ``py.typed`` marker.

Without this marker, mypy treats every symbol imported from ``michelangelo.uniflow``
as ``Any`` once the package is resolved the way a real downstream consumer would see
it — as an installed dependency, not as source code sitting next to the project
being type-checked.

That distinction is easy to get wrong when writing a test for it, because mypy can
find this repo's own ``michelangelo/`` source tree through more than one path at
once: by walking upward from whatever file it's checking (so checking a consumer
file at its real, repo-committed path "finds" the adjacent source regardless of
anything else), and, separately, through this repo's editable install, whose `.pth`
file puts `python/` directly on `sys.path`. The check below closes both: it checks
a *copy* of the consumer file placed outside the repo (so there's nothing for the
upward walk to find), with the editable install's `sys.path` entry filtered out (so
the normal import-resolution path doesn't find it either).

This file does **not** include an automated test proving the positive case would
fail if the marker were removed, after real effort to build one. Removing the
marker and re-running the exact same checks, from inside this pytest session, kept
succeeding regardless of the two fixes above and several more tried on top of them —
closing `sys.modules` entries, removing pytest's assertion-rewrite hook from
`sys.meta_path`, calling `importlib.invalidate_caches()`, passing
`--no-site-packages`. Something about mypy's resolution in an *editable* install
specifically still finds the source tree even with all of that closed, and this
investigation could not pin down what.

The discrimination is real, just not reproduced automatically from here: built an
actual wheel twice (once with the marker, once without, via `python -m build`),
installed each into a fresh venv with no editable install involved, and ran
`python -m mypy` as a plain subprocess from `/tmp` against a copy of the exact
consumer file used below. The no-marker wheel produced two real errors (`Class
cannot subclass "TaskConfig" (has type "Any")`, `Expression is of type "Any", not
"bool"`); the marker-bearing wheel passed cleanly. If this test suite ever needs an
automated version of that proof, do it that way — a real built wheel in a fresh
venv, checked by a subprocess `mypy` call, never an editable install or an
in-process `mypy.api.run()` — rather than retrying the in-process approach.

``test_decorator_typing.py``'s mypy test does not hit either problem: it
deliberately points ``MYPYPATH`` at the source tree on purpose, to test the
decorators' annotations specifically and independent of py.typed. This test is
checking the opposite thing — what a consumer sees without that override.
"""

import sys
from pathlib import Path

import pytest

_CONSUMER = Path(__file__).parent / "typing_samples" / "py_typed_consumer.py"
_REPO_PYTHON_ROOT = Path(__file__).resolve().parents[3]


def test_marker_file_exists():
    """The marker ships as a real, empty file at the expected path."""
    import michelangelo.uniflow

    marker = Path(michelangelo.uniflow.__path__[0]) / "py.typed"
    assert marker.is_file()
    assert marker.read_text() == ""


def test_consumer_type_checks_as_an_installed_dependency(tmp_path, monkeypatch):
    """A consumer of Context, Future and TaskConfig passes mypy --strict.

    Resolved the way a real downstream consumer would see the package — as
    something installed, not as source code mypy can walk up into or find on
    sys.path via this repo's own editable install. See the module docstring.
    """
    mypy_api = pytest.importorskip("mypy.api")
    monkeypatch.delenv("MYPYPATH", raising=False)
    monkeypatch.setattr(
        sys,
        "path",
        [p for p in sys.path if Path(p or ".").resolve() != _REPO_PYTHON_ROOT],
    )
    copy = tmp_path / "py_typed_consumer.py"
    copy.write_text(_CONSUMER.read_text())
    stdout, stderr, status = mypy_api.run(
        [
            "--strict",
            "--no-incremental",
            "--follow-imports=silent",
            "--ignore-missing-imports",
            "--cache-dir",
            str(tmp_path / ".mypy_cache"),
            str(copy),
        ]
    )
    assert status == 0, stdout + stderr


def test_other_uniflow_subpackages_have_no_marker_of_their_own():
    """The marker is scoped to uniflow/, not the whole michelangelo package.

    A package-wide marker would expose every public symbol (api/, gen/'s
    generated protobuf stubs with no .pyi files, and every undocumented
    function) to downstream strict type-checking at once — see #2464 for the
    measured error count that would produce. Catches an accidental widening
    (or an accidental move of the marker to the wrong directory) early.
    """
    import michelangelo

    root = Path(michelangelo.__path__[0])
    assert not (root / "py.typed").exists()
    assert (root / "uniflow" / "py.typed").is_file()
