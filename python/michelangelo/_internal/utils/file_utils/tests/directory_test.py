import os
from unittest import TestCase

from michelangelo._internal.utils.file_utils import cd


class DirectoryTest(TestCase):
    def setUp(self):
        os.makedirs("test_dir")

    def tearDown(self):
        os.rmdir("test_dir")

    def test_cd(self):
        test_dir_path = os.path.abspath("test_dir")
        with cd("test_dir"):
            self.assertEqual(os.getcwd(), test_dir_path)
        self.assertEqual(os.getcwd(), os.path.abspath("."))
