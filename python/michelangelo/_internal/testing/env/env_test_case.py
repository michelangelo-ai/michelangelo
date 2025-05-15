import os
from unittest import TestCase


class EnvTestCase(TestCase):
    def setUp(self):
        """
        Preserve the original environment variables.
        """
        self.env = {}
        for key in os.environ:
            self.env[key] = os.environ[key]

    def tearDown(self):
        """
        Restore the original environment variables.
        """
        for key in os.environ:
            if key not in self.env:
                del os.environ[key]
        for key in self.env:
            os.environ[key] = self.env[key]
