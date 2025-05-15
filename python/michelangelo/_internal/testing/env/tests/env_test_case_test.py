import os
import uuid
from unittest import TestCase
from michelangelo.lib._internal.testing.env import EnvTestCase


class EnvTestCaseTest(TestCase):
    def setUp(self):
        self.env_key = "ENV_" + str(uuid.uuid4()).replace("-", "_")
        os.environ[self.env_key] = "test"

    def tearDown(self):
        if self.env_key in os.environ:
            del os.environ[self.env_key]

    def test_setup_and_teardown(self):
        env_test_case = EnvTestCase()
        env_test_case.setUp()
        self.assertIn(self.env_key, os.environ)
        self.assertIn(self.env_key, env_test_case.env)
        self.assertEqual(env_test_case.env[self.env_key], "test")

        os.environ[self.env_key] = "new_test"
        new_env_key = "ENV_" + str(uuid.uuid4()).replace("-", "_")
        os.environ[new_env_key] = "new_test_1"
        env_test_case.tearDown()
        self.assertIn(self.env_key, os.environ)
        self.assertEqual(os.environ[self.env_key], "test")
        self.assertNotIn(new_env_key, os.environ)


class Test(EnvTestCase):
    def setUp(self):
        super().setUp()

    def tearDown(self):
        super().tearDown()
