#!/usr/bin/env python3
"""
Starlark Workflow Runner

A Python script that replicates the functionality of run-star.sh for running
Starlark files as workflows on Cadence or Temporal using the Starlark Worker.

Usage:
    starlark run ./go/worker/starlark/testdata/ping.star
    starlark run ./go/worker/starlark/testdata/ping.star --workflow temporal

More information available at https://github.com/michelangelo-ai/michelangelo/blob/main/go/worker/starlark/README.md.
"""

import argparse
import base64
import os
import shutil
import subprocess
import sys
import tempfile
from typing import Optional


class Logger:
    """Simple logger with emoji prefixes matching the shell script."""

    @staticmethod
    def info(message: str) -> None:
        print(f"ℹ️ {message}", file=sys.stderr)

    @staticmethod
    def success(message: str) -> None:
        print(f"✅ {message}", file=sys.stderr)

    @staticmethod
    def error(message: str) -> None:
        print(f"🟥 ERROR: {message}", file=sys.stderr)


class StarlarkRunner:
    """Main class for running Starlark workflows."""

    def __init__(self):
        self.logger = Logger()
        self.temp_dir: Optional[str] = None

    def _cleanup(self) -> None:
        """Clean up temporary directory."""
        if self.temp_dir and os.path.exists(self.temp_dir):
            self.logger.info("Cleaning up temporary directory ...")
            shutil.rmtree(self.temp_dir)
            self.temp_dir = None

    def _create_package(self, file_path: str, workflow_type: str) -> str:
        """
        Create a package from the Starlark file using Bazel.

        Args:
            file_path: Path to the .star file
            workflow_type: Either 'cadence' or 'temporal'

        Returns:
            Path to the created package file

        Raises:
            RuntimeError: If package creation fails
        """
        self.logger.info("Creating package ...")

        # Determine the correct workflow client target based on workflow type
        if workflow_type == "cadence":
            target = (
                "@com_github_cadence_workflow_starlark_worker//cmd/cadence_client_main"
            )
        else:
            target = (
                "@com_github_cadence_workflow_starlark_worker//cmd/temporal_client_main"
            )

        package_path = os.path.join(self.temp_dir, "package.tar.gz")

        # Build the bazel command
        cmd = [
            "bazel",
            "run",
            f"--run_under=cd {os.getcwd()} &&",
            target,
            "--",
            "package",
            "--file",
            file_path,
        ]

        try:
            with open(package_path, "wb") as f:
                result = subprocess.run(
                    cmd, stdout=f, stderr=subprocess.PIPE, text=True
                )

            if result.returncode != 0:
                self.logger.error(f"Failed to create package: {result.stderr}")
                raise RuntimeError("Package creation failed")

            return package_path

        except subprocess.SubprocessError as e:
            self.logger.error(f"Failed to create package: {e}")
            raise RuntimeError("Package creation failed")

    def _encode_package(self, package_path: str) -> str:
        """
        Encode the package file as base64.

        Args:
            package_path: Path to the package file

        Returns:
            Base64 encoded package content
        """
        try:
            with open(package_path, "rb") as f:
                package_content = f.read()

            return base64.b64encode(package_content).decode("utf-8")

        except Exception as e:
            self.logger.error(f"Failed to encode package: {e}")
            raise RuntimeError("Package encoding failed")

    def _create_input_file_cadence(self, package_b64: str, file_path: str) -> str:
        """
        Create input file for Cadence workflow.

        Args:
            package_b64: Base64 encoded package
            file_path: Original file path

        Returns:
            Path to the created input file
        """
        input_file = os.path.join(self.temp_dir, "input.json")

        # Create the input format matching the shell script
        input_content = f'"{package_b64}"\n"{file_path}"\n"main"\n[]\n[]\n{{}}'

        with open(input_file, "w") as f:
            f.write(input_content)

        return input_file

    def _run_cadence_workflow(self, file_path: str) -> None:
        """
        Run Starlark file as Cadence workflow.

        Args:
            file_path: Path to the .star file
        """
        # Create temporary directory
        self.temp_dir = tempfile.mkdtemp()
        self.logger.info(f"Temporary directory created at: {self.temp_dir}")

        try:
            package_path = self._create_package(file_path, "cadence")

            self.logger.info("Creating input file ...")
            package_b64 = self._encode_package(package_path)

            input_file = self._create_input_file_cadence(package_b64, file_path)

            cadence_args = ["cadence"]
            if os.getenv("UFC_CADENCE_ENV"):
                cadence_args.extend(["--env", os.getenv("UFC_CADENCE_ENV")])

            if os.getenv("UFC_CADENCE_PROXY_REGION"):
                cadence_args.extend(
                    ["--proxy_region", os.getenv("UFC_CADENCE_PROXY_REGION")]
                )
            else:
                self.logger.info(
                    "No proxy region set, using transport and address from environment"
                )
                if os.getenv("UFC_CADENCE_TRANSPORT") and os.getenv(
                    "UFC_CADENCE_ADDRESS"
                ):
                    cadence_args.extend(
                        [
                            "--transport",
                            os.getenv("UFC_CADENCE_TRANSPORT"),
                            "--address",
                            os.getenv("UFC_CADENCE_ADDRESS"),
                        ]
                    )
                else:
                    cadence_args.extend(
                        [
                            "--address",
                            os.getenv("UFC_CADENCE_ADDRESS", "localhost:7933"),
                        ]
                    )

            # Add domain and other required parameters
            cadence_args.extend(
                [
                    "--domain",
                    os.getenv("UFC_CADENCE_DOMAIN", "default"),
                    "workflow",
                    "run",
                    "--tasklist",
                    os.getenv("UFC_CADENCE_TASK_LIST", "default"),
                    "--workflow_type",
                    "starlark-worklow",
                    "--execution_timeout",
                    "3600",
                    "--input_file",
                    input_file,
                ]
            )

            # Run the workflow
            self.logger.info("Running workflow ...")
            subprocess.run(cadence_args, check=True)

            self.logger.success("Workflow executed successfully")

        finally:
            self._cleanup()

    def _run_temporal_workflow(self, file_path: str) -> None:
        """
        Run Starlark file as Temporal workflow.

        Args:
            file_path: Path to the .star file
        """
        # Create temporary directory
        self.temp_dir = tempfile.mkdtemp()
        self.logger.info(f"Temporary directory created at: {self.temp_dir}")

        try:
            package_path = self._create_package(file_path, "temporal")

            self.logger.info("Creating input file ...")
            package_b64 = self._encode_package(package_path)

            temporal_args = ["/opt/homebrew/bin/temporal"]
            temporal_args.extend(
                ["--address", os.getenv("UFC_TEMPORAL_ADDRESS", "localhost:7233")]
            )
            temporal_args.extend(
                ["--namespace", os.getenv("UFC_TEMPORAL_NAMESPACE", "default")]
            )

            if os.getenv("UFC_TEMPORAL_ENV"):
                temporal_args.extend(["--env", os.getenv("UFC_TEMPORAL_ENV")])

            # add other necessary arguments
            temporal_args.extend(
                [
                    "workflow",
                    "start",
                    "--task-queue",
                    os.getenv("UFC_TEMPORAL_TASK_QUEUE", "default"),
                    "--type",
                    os.getenv("UFC_TEMPORAL_WORKFLOW_TYPE", "starlark-worklow"),
                    "--execution-timeout",
                    "3600s",
                    "--input",
                    f'"{package_b64}"',
                    "--input",
                    f'"{file_path}"',
                    "--input",
                    '"main"',
                    "--input",
                    "[]",
                    "--input",
                    "[]",
                    "--input",
                    "{}",
                ]
            )

            self.logger.info("Running workflow ...")
            subprocess.run(temporal_args, check=True)

            self.logger.success("Workflow executed successfully")

        finally:
            self._cleanup()

    def run(self, file_path: str, workflow_type: str = "cadence") -> None:
        """
        Run a Starlark file as a workflow.

        Args:
            file_path: Path to the .star file
            workflow_type: Either 'cadence' or 'temporal'
        """
        # Validate file exists
        if not os.path.exists(file_path):
            self.logger.error(f"File not found: {file_path}")
            sys.exit(1)

        # Validate file extension
        if not file_path.endswith(".star"):
            self.logger.error(f"File must have .star extension: {file_path}")
            sys.exit(1)

        try:
            if workflow_type == "cadence":
                self._run_cadence_workflow(file_path)
            elif workflow_type == "temporal":
                self._run_temporal_workflow(file_path)
            else:
                self.logger.error(f"Invalid workflow type: {workflow_type}")
                sys.exit(1)

        except RuntimeError as e:
            self.logger.error(str(e))
            sys.exit(1)
        except subprocess.CalledProcessError as e:
            self.logger.error(f"Command failed with exit code {e.returncode}")
            sys.exit(1)
        except KeyboardInterrupt:
            self.logger.info("Interrupted by user")
            sys.exit(1)
        except Exception as e:
            self.logger.error(f"Unexpected error: {e}")
            sys.exit(1)


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Run Starlark files as workflows on Cadence or Temporal",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s run ./go/worker/starlark/testdata/ping.star
  %(prog)s run ./go/worker/starlark/testdata/ping.star --workflow temporal

Environment Variables:
  Cadence:
    UFC_CADENCE_TASK_LIST      Task list (default: "default")
    UFC_CADENCE_DOMAIN         Domain (default: "default")
    UFC_CADENCE_PROXY_REGION   Proxy region
    UFC_CADENCE_TRANSPORT      Transport protocol
    UFC_CADENCE_ADDRESS        Address (default: "localhost:7933")
    UFC_CADENCE_ENV            Environment

  Temporal:
    UFC_TEMPORAL_ADDRESS       Address (default: "localhost:7233")
    UFC_TEMPORAL_NAMESPACE     Namespace (default: "default")
    UFC_TEMPORAL_ENV           Environment
    UFC_TEMPORAL_TASK_QUEUE    Task queue (default: "default")
    UFC_TEMPORAL_WORKFLOW_TYPE Workflow type (default: "starlark-worklow")
        """,
    )

    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Run command
    run_parser = subparsers.add_parser("run", help="Run a Starlark file as a workflow")
    run_parser.add_argument("file", help="Path to the .star file to execute")
    run_parser.add_argument(
        "--workflow",
        choices=["cadence", "temporal"],
        default="cadence",
        help="Workflow engine to use (default: cadence)",
    )

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        sys.exit(1)

    if args.command == "run":
        runner = StarlarkRunner()
        runner.run(args.file, args.workflow)


if __name__ == "__main__":
    main()
