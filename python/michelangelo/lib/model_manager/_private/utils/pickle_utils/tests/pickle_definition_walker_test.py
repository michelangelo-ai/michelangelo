import os
import pickle
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.pickle_utils import (
    walk_pickle_definitions_in_dir,
)
from michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package import (  # noqa: E501
    A,
    func,
)
from michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep import (  # noqa: E501
    B,
)


class TestClass:
    """Fixture class used in pickle definition tests."""


class PickleDefinitionWalkerTest(TestCase):
    """Tests walking directories to find pickle definitions."""

    def create_pickle_files(self, directory: str):
        """Create pickle fixtures in a temporary directory tree."""
        subdir1 = os.path.join(directory, "subdir1")
        subdir2 = os.path.join(directory, "subdir2")
        subsubdir1 = os.path.join(subdir1, "subsubdir1")
        os.makedirs(subsubdir1)
        os.makedirs(subdir2)

        with open(os.path.join(subdir1, "file1.pkl"), "wb") as f:
            pickle.dump(TestClass(), f)

        with open(os.path.join(subdir1, "file2.pkl"), "wb") as f:
            pickle.dump(func, f)

        with open(os.path.join(subsubdir1, "file3.pkl"), "wb") as f:
            pickle.dump(A(), f)

        with open(os.path.join(subsubdir1, "file4.pkl"), "wb") as f:
            pickle.dump(B(1), f)

        with open(os.path.join(subsubdir1, "file5.pkl"), "wb") as f:
            pickle.dump({"A": A(), "TestClass": TestClass}, f)

    def test_walk_pickle_definitions_in_dir(self):
        """It walks directories and finds pickled symbol references."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self.create_pickle_files(temp_dir)

            defs = set(walk_pickle_definitions_in_dir(temp_dir))

            self.assertEqual(
                defs,
                {
                    (
                        "pickle_definition_walker_test",
                        "TestClass",
                        os.path.join(temp_dir, "subdir1", "file1.pkl"),
                    ),
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "func",
                        os.path.join(temp_dir, "subdir1", "file2.pkl"),
                    ),
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file3.pkl"),
                    ),
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep",
                        "B",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file4.pkl"),
                    ),
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                    (
                        "pickle_definition_walker_test",
                        "TestClass",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                },
            )

    def test_walk_pickle_definitions_in_dir_with_ignore(self):
        """It supports filtering out matched pickle definitions."""
        with tempfile.TemporaryDirectory() as temp_dir:
            self.create_pickle_files(temp_dir)

            defs = set(
                walk_pickle_definitions_in_dir(
                    temp_dir,
                    match=lambda m, a, f: m.endswith("mod")
                    and a == "A"
                    and "subsubdir1" in f,
                )
            )

            self.assertEqual(
                defs,
                {
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file3.pkl"),
                    ),
                    (
                        "michelangelo.lib.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                },
            )
