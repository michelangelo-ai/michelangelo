import os
import pickle
import tempfile
from unittest import TestCase

from michelangelo.lib.model_manager._private.utils.pickle_utils import (
    find_pickled_files,
    is_pickled_file,
)


class PickledFileTest(TestCase):
    """Tests identification of pickled files on disk."""

    def test_is_pickled_file(self):
        """It detects whether a file contains pickle contents."""
        a = 1
        with tempfile.TemporaryDirectory() as temp_dir:
            fn = os.path.join(temp_dir, "test.pkl")

            with open(fn, "wb") as f:
                pickle.dump(a, f)

            self.assertTrue(is_pickled_file(fn))

            with open(fn, "w") as f:
                f.write("not a pickle")

            self.assertFalse(is_pickled_file(fn))

    def test_find_pickled_files(self):
        """It finds pickled files within directory trees."""
        with tempfile.TemporaryDirectory() as temp_dir:
            subdir1 = os.path.join(temp_dir, "subdir1")
            subsubdir1 = os.path.join(subdir1, "subdir1")
            subdir2 = os.path.join(temp_dir, "subdir2")
            os.makedirs(subdir1)
            os.makedirs(subsubdir1)
            os.makedirs(subdir2)
            fn1 = os.path.join(temp_dir, "test1.pkl")
            fn2 = os.path.join(temp_dir, "test2.txt")
            fn3 = os.path.join(subdir1, "test3.pkl")
            fn4 = os.path.join(subdir1, "test4.txt")
            fn5 = os.path.join(subsubdir1, "test5.pkl")
            fn6 = os.path.join(subdir2, "test6.pkl")

            with open(fn1, "wb") as f:
                pickle.dump(1, f)

            with open(fn2, "w") as f:
                f.write("not a pickle")

            with open(fn3, "wb") as f:
                pickle.dump(1, f)

            with open(fn4, "w") as f:
                f.write("not a pickle")

            with open(fn5, "wb") as f:
                pickle.dump(1, f)

            with open(fn6, "wb") as f:
                pickle.dump(1, f)

            files = set(find_pickled_files(temp_dir))
            self.assertEqual(files, {fn1, fn3, fn5, fn6})
