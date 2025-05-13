from unittest import TestCase


class HDFSPathsTest(TestCase):
    def test_hdfs_paths(self):
        from michelangelo.lib.model_manager._private.constants.hdfs_paths import HDFS_TMP_MODELS_DIR

        self.assertEqual(HDFS_TMP_MODELS_DIR, "/tmp/michelangelo/models")
