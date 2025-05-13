from unittest import TestCase
from uber.ai.michelangelo.shared.errors.hdfs_error import (
    HDFSError,
)


class HDFSErrorTest(TestCase):
    def test_hdfs_error(self):
        with self.assertRaises(HDFSError):
            raise HDFSError("test")
