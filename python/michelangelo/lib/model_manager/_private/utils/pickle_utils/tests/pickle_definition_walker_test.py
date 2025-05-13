import os
import pickle
import tempfile
from unittest import TestCase
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils import walk_pickle_definitions_in_dir
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package import A, func
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep import B


class TestClass:
    pass


class PickleDefinitionWalkerTest(TestCase):
    def create_pickle_files(self, directory: str):
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
        with tempfile.TemporaryDirectory() as temp_dir:
            self.create_pickle_files(temp_dir)

            defs = set(walk_pickle_definitions_in_dir(temp_dir))

            self.assertEqual(
                defs,
                {
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.pickle_definition_walker_test",
                        "TestClass",
                        os.path.join(temp_dir, "subdir1", "file1.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "func",
                        os.path.join(temp_dir, "subdir1", "file2.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file3.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep",
                        "B",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file4.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.pickle_definition_walker_test",
                        "TestClass",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                },
            )

    def test_walk_pickle_definitions_in_dir_with_ignore(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            self.create_pickle_files(temp_dir)

            defs = set(walk_pickle_definitions_in_dir(temp_dir, match=lambda m, a, f: m.endswith("mod") and a == "A" and "subsubdir1" in f))

            self.assertEqual(
                defs,
                {
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file3.pkl"),
                    ),
                    (
                        "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod",
                        "A",
                        os.path.join(temp_dir, "subdir1", "subsubdir1", "file5.pkl"),
                    ),
                },
            )
