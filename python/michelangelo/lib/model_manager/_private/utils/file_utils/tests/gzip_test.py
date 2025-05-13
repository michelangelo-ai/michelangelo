from unittest import TestCase
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import (
    gzip_compress,
    gzip_decompress,
)
import os


class GzipTest(TestCase):
    def test_gzip_compress_and_decompress(self):
        src_file = "test.txt"
        dest_file = "test.txt.gz"
        with open(src_file, "w") as f:
            f.write("test")

        gzip_compress(src_file, dest_file)
        self.assertTrue(os.path.exists(dest_file))

        gzip_decompress(dest_file, src_file)
        self.assertTrue(os.path.exists(src_file))
        with open(src_file) as f:
            self.assertEqual(f.read(), "test")

        os.remove(src_file)
        os.remove(dest_file)
