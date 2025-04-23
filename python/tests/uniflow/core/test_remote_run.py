import os
import base64
import json
import unittest
from unittest.mock import patch
from michelangelo.uniflow.core.remote_run import RemoteRun
from michelangelo.uniflow.core import workflow

_test_env = {
    "UFC_CADENCE_DOMAIN": "default",
    "UFC_CADENCE_TASK_LIST": "default",
    "UFC_CADENCE_WORKFLOW_TYPE": "starlark",
}


@workflow()
def my_workflow(spec: dict):
    """
    Minimal Uniflow workflow function for testing purposes.
    """
    print(spec)
    return 0


class Test(unittest.TestCase):
    @patch("subprocess.run")
    @patch.dict(os.environ, _test_env)
    def test_main_minimal_command(self, subprocess_run):
        """
        This test case verifies the remote run command with a minimal set of arguments.
        """
        # Provide a minimal set of the arguments required to run the workflow in the remote mode.

        rr = RemoteRun(
            fn=my_workflow,
            image="test-container-registry:8000/test-image:latest",
            storage_url="hdfs://test_storage",
        )
        rr.yes = True
        rr.run()

        subprocess_run.assert_called_once()

        cmd = subprocess_run.call_args.args[0]  # The downstream subprocess command.
        self.assertIsInstance(cmd, list)

        # Assert the command.
        # Ensure the command runs the Cadence CLI with the "workflow start" subcommand.
        self.assertEqual(cmd[0], "cadence")
        self.assertEqual(cmd.index("workflow"), cmd.index("start") - 1)

        # Parse command line arguments
        domain = cmd[cmd.index("--domain") + 1]
        tasklist = cmd[cmd.index("--tasklist") + 1]
        workflow_type = cmd[cmd.index("--workflow_type") + 1]
        execution_timeout = cmd[cmd.index("--execution_timeout") + 1]
        workflow_id = cmd[cmd.index("--workflow_id") + 1]
        workflow_input = cmd[cmd.index("--input") + 1]

        # Assert Cadence Service configuration
        self.assertEqual("default", domain)
        self.assertEqual("default", tasklist)
        self.assertEqual("starlark", workflow_type)
        self.assertTrue(execution_timeout.isdigit())
        self.assertTrue(my_workflow.__name__ in workflow_id)

        # Parse workflow input further
        tarball, entrypoint_file, entrypoint_fn, args, keywords, env, _ = (
            workflow_input.split("\n")
        )

        # Assert input parts. All parts must be JSON serialized.
        # Tarball is a JSON string containing tarball bytes in base64.
        self.assertTrue(tarball.startswith('"'))
        self.assertTrue(tarball.endswith('"'))
        self.assertTrue(base64.b64decode(tarball[1:-1], validate=True))

        # Entrypoint information is not required in the input because the tarball contains the default entrypoint info,
        # such as entrypoint file and function name.
        self.assertEqual('""', entrypoint_file)
        self.assertEqual('""', entrypoint_fn)

        # By default, there are no arguments nor keywords provided for the workflow function execution.
        self.assertEqual("[]", args)  # Empty argument list[Any]
        self.assertEqual("[]", keywords)  # Empty keyword list[tuple[str, Any]]

        # User did not provide any environment variables, however, the task image and storage URL are set by the CLI.
        env_dict = json.loads(env)
        expected_env = {
            "UF_TASK_IMAGE": "test-container-registry:8000/test-image:latest",
            "UF_STORAGE_URL": "hdfs://test_storage",
        }
        self.assertDictEqual(expected_env, env_dict)

    @patch("subprocess.run")
    @patch.dict(os.environ, _test_env)
    def test_main_workflow_args(self, subprocess_run):
        """
        This test case verifies that user-provided arguments and environment variables are properly encoded
        and passed to the Cadence workflow execution input.
        """
        # Run my_workflow with arguments and environment variables.
        # Users may provide multiple arguments encoded in JSON. Example:
        #
        #       --args '[1, 2, 3]' '{"key": "value"}' '3.14' 'true' '"test"' 'null'
        #
        # Users may provide multiple environment variables encoded as KEY=VALUE. Example:
        #
        #       --env 'FOO=BAR' 'FIZ=BAZ'

        rr = RemoteRun(
            fn=my_workflow,
            image="test-container-registry:8000/test-image:latest",
            storage_url="hdfs://test_storage",
        )
        rr.args = [{"key": "value"}, "test"]
        rr.environ.update(
            {
                "FOO": "BAR",
                "FIZ": "BAZ",
            }
        )
        rr.yes = True
        rr.run()

        subprocess_run.assert_called_once()

        cmd = subprocess_run.call_args.args[0]  # The downstream subprocess command.
        self.assertIsInstance(cmd, list)

        # Parse Cadence workflow execution input
        workflow_input = cmd[cmd.index("--input") + 1]
        _, _, _, args, _, env, _ = workflow_input.split("\n")

        # Assert that user-provided arguments are passed to the workflow input.
        args_list = json.loads(args)
        expected_args = [
            {"key": "value"},
            "test",
        ]
        self.assertEqual(expected_args, args_list)

        # Assert that user-provided environment variables are passed to the workflow input.
        # Expected environment variables: CLI-provided + user-provided.
        env_dict = json.loads(env)
        expected_env = {
            "UF_TASK_IMAGE": "test-container-registry:8000/test-image:latest",
            "UF_STORAGE_URL": "hdfs://test_storage",
            "FOO": "BAR",
            "FIZ": "BAZ",
        }
        self.assertEqual(expected_env, env_dict)
