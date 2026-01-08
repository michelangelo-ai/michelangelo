#!/usr/bin/env python3
"""Demo script showing YAML workflow execution with uniflow.

This script demonstrates how to use the new YAML workflow functionality
with both local and remote execution modes.

Usage:
    # Local execution
    python yaml_workflow_demo.py yaml-local-run example_workflow.yml

    # Remote execution (requires storage and image)
    python yaml_workflow_demo.py yaml-remote-run example_workflow.yml \
        --storage-url s3://ml-bucket/storage \
        --image ml-training:v1.0

    # Validate YAML workflow without running
    python yaml_workflow_demo.py validate example_workflow.yml
"""

import sys
import os
import logging
from pathlib import Path

# Add the current directory to path so we can import example_tasks
sys.path.insert(0, str(Path(__file__).parent))

# Import uniflow components
import michelangelo.uniflow as uniflow
from michelangelo.uniflow.core.yaml_parser import validate_yaml_workflow

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
log = logging.getLogger(__name__)


def main():
    """Main entry point for the demo script."""
    if len(sys.argv) < 2:
        print_usage()
        sys.exit(1)

    command = sys.argv[1]

    if command == "validate":
        if len(sys.argv) < 3:
            print("Usage: python yaml_workflow_demo.py validate <yaml_file>")
            sys.exit(1)
        validate_workflow(sys.argv[2])

    elif command in ("yaml-local-run", "yaml-remote-run"):
        if len(sys.argv) < 3:
            print(f"Usage: python yaml_workflow_demo.py {command} <yaml_file> [options]")
            sys.exit(1)
        run_workflow(sys.argv[1], sys.argv[2])

    else:
        print_usage()
        sys.exit(1)


def print_usage():
    """Print usage information."""
    print("""
Uniflow YAML Workflow Demo

Commands:
    validate <yaml_file>                 - Validate YAML workflow syntax
    yaml-local-run <yaml_file>          - Execute workflow locally
    yaml-remote-run <yaml_file> [opts]  - Execute workflow remotely

Examples:
    # Validate workflow
    python yaml_workflow_demo.py validate example_workflow.yml

    # Run locally
    python yaml_workflow_demo.py yaml-local-run example_workflow.yml

    # Run remotely
    python yaml_workflow_demo.py yaml-remote-run example_workflow.yml \\
        --storage-url s3://ml-bucket/storage \\
        --image ml-training:v1.0

Remote Run Options:
    --storage-url URL    - Storage URL for workflow data
    --image IMAGE        - Container image for tasks
    --workflow ENGINE    - Workflow engine (cadence/temporal)
    --cron EXPR          - Cron schedule for periodic runs
    --file-sync          - Sync local code changes
    --yes                - Skip confirmation prompts

Environment Variables:
    UF_STORAGE_URL       - Default storage URL
    UF_TASK_IMAGE        - Default task image
""")


def validate_workflow(yaml_file):
    """Validate a YAML workflow file."""
    try:
        log.info(f"Validating YAML workflow: {yaml_file}")

        if not Path(yaml_file).exists():
            log.error(f"YAML file not found: {yaml_file}")
            sys.exit(1)

        # Validate the workflow
        validate_yaml_workflow(yaml_file)
        log.info("✅ YAML workflow validation successful!")

        # Show workflow summary
        from michelangelo.uniflow.core.yaml_parser import YAMLWorkflowParser
        parser = YAMLWorkflowParser()
        config = parser.parse_file(yaml_file)

        print(f"\nWorkflow Summary:")
        print(f"  Name: {config.metadata.name}")
        print(f"  Description: {config.metadata.description}")
        print(f"  Version: {config.metadata.version}")
        print(f"  Tasks: {len(config.tasks)}")

        # Show task overview
        print(f"\nTasks:")
        for task_name, task_spec in config.tasks.items():
            task_type = "Static"
            if task_spec.expand:
                task_type = "Dynamic (Expand)"
            elif task_spec.condition:
                task_type = "Conditional"
            elif task_spec.collect:
                task_type = "Collector"

            print(f"  {task_name:20} - {task_type:18} - {task_spec.description or 'No description'}")

    except Exception as e:
        log.error(f"❌ Validation failed: {e}")
        sys.exit(1)


def run_workflow(command, yaml_file):
    """Run a YAML workflow using uniflow context."""
    try:
        log.info(f"Running YAML workflow: {yaml_file}")

        if not Path(yaml_file).exists():
            log.error(f"YAML file not found: {yaml_file}")
            sys.exit(1)

        # Override sys.argv to match what uniflow's create_context expects
        original_argv = sys.argv.copy()
        sys.argv = [sys.argv[0], command] + sys.argv[3:]  # Remove the yaml_file argument since it's now the fn parameter

        try:
            # Create context and run workflow
            ctx = uniflow.create_context()

            log.info(f"Execution mode: {'Local' if ctx.is_local_run() else 'Remote'}")
            log.info(f"YAML workflow: {ctx.is_yaml_workflow()}")

            # Execute the workflow
            result = ctx.run(yaml_file)

            log.info("✅ Workflow execution completed successfully!")

            if result:
                print(f"\nWorkflow Results:")
                if isinstance(result, dict):
                    for task_name, task_result in result.items():
                        print(f"  {task_name}: {type(task_result).__name__}")
                else:
                    print(f"  Result: {result}")

        finally:
            # Restore original sys.argv
            sys.argv = original_argv

    except Exception as e:
        log.error(f"❌ Workflow execution failed: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()