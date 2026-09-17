"""Tests for the sandbox model-sync resource script."""

import io
import runpy
import tarfile
import tempfile
from pathlib import Path
from unittest import TestCase

_SCRIPT = (
    Path(__file__).resolve().parents[3]
    / "michelangelo"
    / "cli"
    / "sandbox"
    / "resources"
    / "sync-models.py"
)
_SAFE_EXTRACTALL = runpy.run_path(str(_SCRIPT))["safe_extractall"]


def _model_archive(member_name="./1/model.pt"):
    archive_data = io.BytesIO()
    with tarfile.open(fileobj=archive_data, mode="w") as archive:
        root = tarfile.TarInfo(".")
        root.type = tarfile.DIRTYPE
        root.mode = 0o755
        archive.addfile(root)

        payload = b"model"
        member = tarfile.TarInfo(member_name)
        member.mode = 0o644
        member.size = len(payload)
        archive.addfile(member, io.BytesIO(payload))
    archive_data.seek(0)
    return archive_data


class SafeExtractAllTest(TestCase):
    """Tests for the Python 3.11 tar extraction fallback."""

    def test_accepts_archive_root_member(self):
        """A root member resolving exactly to the destination is valid."""
        with tempfile.TemporaryDirectory() as destination:
            with tarfile.open(fileobj=_model_archive()) as archive:
                _SAFE_EXTRACTALL(archive, destination)

            model_path = Path(destination) / "1" / "model.pt"
            self.assertEqual(model_path.read_bytes(), b"model")

    def test_rejects_parent_traversal(self):
        """A member outside the destination remains blocked."""
        with tempfile.TemporaryDirectory() as parent:
            destination = Path(parent) / "model"
            with (
                tarfile.open(fileobj=_model_archive("../escape.txt")) as archive,
                self.assertRaisesRegex(ValueError, "path escapes"),
            ):
                _SAFE_EXTRACTALL(archive, str(destination))

            self.assertFalse((Path(parent) / "escape.txt").exists())
