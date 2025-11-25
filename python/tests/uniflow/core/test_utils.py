import os
import unittest
from dataclasses import dataclass, is_dataclass
from pathlib import Path
from typing import Any, Optional
from unittest.mock import MagicMock, patch

import fsspec
import fsspec.core

from michelangelo.uniflow.core.utils import (
    dataclass_dict,
    encode_value_to_json,
    is_dataclass_instance,
)

pwd = os.environ["PWD"]


class Test(unittest.TestCase):
    def test_fsspec_url_to_fs_no_scheme(self):
        _, absolute_path = fsspec.core.url_to_fs("/host/path/data.json")
        _, relative_path = fsspec.core.url_to_fs("host/path/data.json")

        self.assertEqual("/host/path/data.json", absolute_path)

        expected_relative_path = Path(pwd) / "host" / "path" / "data.json"
        self.assertEqual(str(expected_relative_path), relative_path)

    def test_fsspec_url_to_fs_file(self):
        _, absolute_path = fsspec.core.url_to_fs("file:///host/path/data.json")
        _, relative_path = fsspec.core.url_to_fs("file://host/path/data.json")

        self.assertEqual("/host/path/data.json", absolute_path)

        expected_relative_path = Path(pwd) / "host" / "path" / "data.json"
        self.assertEqual(str(expected_relative_path), relative_path)

    def test_fsspec_url_to_fs_memory(self):
        # memory - absolute only path
        _, path1 = fsspec.core.url_to_fs("memory:///host/path/data.json")
        _, path2 = fsspec.core.url_to_fs("memory://host/path/data.json")

        expected_path = "/host/path/data.json"
        self.assertEqual(expected_path, path1)
        self.assertEqual(expected_path, path2)

    def test_encode_value_to_json(self):
        # Mocking tempfile.NamedTemporaryFile
        mock_temp_file = MagicMock()
        mock_file = MagicMock()
        mock_temp_file.__enter__.return_value = mock_file
        with patch("tempfile.NamedTemporaryFile", mock_temp_file):
            encode_value_to_json("test_value")


@dataclass
class Resource:  # basic dataclass
    index: int
    path: str
    metadata: Optional[Any] = None


class DataclassTestCase(unittest.TestCase):
    def test_dataclass_dict_required_only_attrs(self):
        # Init resource with required only attributes
        resource = Resource(
            index=101,
            path="/resources/101",
        )
        dct = dataclass_dict(resource)
        expected = {
            "index": 101,
            "path": "/resources/101",
            "metadata": None,
        }
        self.assertEqual(expected, dct)

    def test_dataclass_dict_non_recursive(self):
        # Init resource with another inner resource in its metadata. The inner resource must not be converted to
        # dictionary because dataclass_dict supposed to be non-recursive.
        resource = Resource(
            index=101,
            path="/resources/101",
            metadata=Resource(
                index=1,
                path="/resources/1",
            ),
        )
        dct = dataclass_dict(resource)
        expected = {
            "index": 101,
            "path": "/resources/101",
            "metadata": Resource(
                index=1,
                path="/resources/1",
            ),
        }
        self.assertEqual(expected, dct)

    def test_is_dataclass_instance(self):
        instance = Resource(index=0, path="")
        self.assertTrue(is_dataclass_instance(instance))
        self.assertFalse(
            is_dataclass_instance(Resource)
        )  # Resource is not a dataclass type, not an instance

        # Standard Python dataclass.is_dataclass returns true for both type and instance.
        self.assertTrue(is_dataclass(instance))
        self.assertTrue(is_dataclass(Resource))
