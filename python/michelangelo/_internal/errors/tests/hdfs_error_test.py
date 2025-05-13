from unittest import TestCase
from michelangelo._internal.errors.hdfs_error import (
    HDFSError,
)


class HDFSErrorTest(TestCase):
    def test_hdfs_error(self):
        with self.assertRaises(HDFSError):
            raise HDFSError("test")
