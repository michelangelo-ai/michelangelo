"""Tests for michelangelo._nightly_warning."""

import warnings
from unittest import mock

from michelangelo._nightly_warning import _check_nightly


class TestNightlyWarning:
    """Verify nightly build warning behavior."""

    def test_warns_on_dev_version(self):
        """Nightly (.dev) version emits a UserWarning."""
        with (
            mock.patch("importlib.metadata.version", return_value="0.3.0.dev20260625"),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            nightly_warnings = [x for x in w if "nightly development build" in str(x.message)]
            assert len(nightly_warnings) == 1
            assert "0.3.0.dev20260625" in str(nightly_warnings[0].message)

    def test_no_warning_on_stable_version(self):
        """Stable version does not emit a warning."""
        with (
            mock.patch("importlib.metadata.version", return_value="0.3.0"),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            nightly_warnings = [x for x in w if "nightly development build" in str(x.message)]
            assert len(nightly_warnings) == 0

    def test_no_warning_when_version_lookup_fails(self):
        """Exception during version lookup is silently caught."""
        with (
            mock.patch("importlib.metadata.version", side_effect=Exception("not found")),
            warnings.catch_warnings(record=True) as w,
        ):
            warnings.simplefilter("always")
            _check_nightly()
            nightly_warnings = [x for x in w if "nightly development build" in str(x.message)]
            assert len(nightly_warnings) == 0
