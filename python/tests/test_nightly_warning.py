"""Tests for michelangelo._nightly_warning."""

import warnings
from unittest import mock

from michelangelo._nightly_warning import _check_nightly

_MARKER = "nightly development build"


def _nightly_warnings(w):
    """Filter warnings list to nightly build warnings."""
    return [x for x in w if _MARKER in str(x.message)]


class TestNightlyWarning:
    """Verify nightly build warning behavior."""

    def test_warns_on_dev_version(self):
        """Nightly (.dev) version emits a UserWarning."""
        with (
            mock.patch(
                "importlib.metadata.version",
                return_value="0.3.0.dev20260625",
            ),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            matched = _nightly_warnings(w)
            assert len(matched) == 1
            assert "0.3.0.dev20260625" in str(matched[0].message)

    def test_no_warning_on_stable_version(self):
        """Stable version does not emit a warning."""
        with (
            mock.patch(
                "importlib.metadata.version",
                return_value="0.3.0",
            ),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            assert len(_nightly_warnings(w)) == 0

    def test_no_warning_when_version_lookup_fails(self):
        """Exception during version lookup is silently caught."""
        with (
            mock.patch(
                "importlib.metadata.version",
                side_effect=Exception("not found"),
            ),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            assert len(_nightly_warnings(w)) == 0
