#!/usr/bin/env python3
"""Script to inspect the generated Python code from YAML workflows."""

import sys
import os
import inspect
import json
from pathlib import Path

# Add the current directory to path so we can import example_tasks
sys.path.insert(0, str(Path(__file__).parent))

import michelangelo.uniflow as uniflow

def inspect_yaml_workflow(yaml_file):
    """Load YAML workflow and show the generated Python code."""
    print(f"\n🔍 Inspecting YAML workflow: {yaml_file}")
    print("=" * 60)

    try:
        # Parse YAML and create workflow function
        parser = uniflow.YAMLWorkflowParser()
        config = parser.parse_file(yaml_file)
        workflow_func = parser.create_workflow_function()

        print(f"\n📋 Workflow Summary:")
        print(f"  Name: {config.metadata.name}")
        print(f"  Description: {config.metadata.description}")
        print(f"  Tasks: {len(config.tasks)}")

        # Show task functions created
        print(f"\n🛠️  Generated Task Functions:")
        for task_name, task_func in parser.task_functions.items():
            task_type = type(task_func).__name__
            print(f"  {task_name:20} -> {task_type}")

            # Show function details
            if hasattr(task_func, 'base_task'):
                print(f"    Base function: {task_func.base_task._fn.__name__}")
                print(f"    Dynamic type: {task_func.dynamic_type}")
            else:
                print(f"    Function: {task_func._fn.__name__}")

            # Show configuration
            print(f"    Config: {type(task_func._config).__name__}")
            if hasattr(task_func._config, 'head_cpu'):
                print(f"      CPU: {task_func._config.head_cpu}")
            if hasattr(task_func._config, 'head_memory'):
                print(f"      Memory: {task_func._config.head_memory}")

        # Show execution order
        print(f"\n📈 Execution Order:")
        execution_order = parser._get_execution_order()
        for i, task_name in enumerate(execution_order, 1):
            print(f"  {i}. {task_name}")

        # Show the generated workflow function
        print(f"\n🐍 Generated Python Workflow Function:")
        print(f"  Function name: {workflow_func.__name__}")
        print(f"  Function type: {type(workflow_func)}")

        # Show the source if possible
        try:
            source = inspect.getsource(workflow_func)
            print(f"  Source code preview:")
            lines = source.split('\n')[:10]  # First 10 lines
            for line in lines:
                if line.strip():
                    print(f"    {line}")
            if len(source.split('\n')) > 10:
                print(f"    ... ({len(source.split('\n'))} total lines)")
        except:
            print(f"  Source: <dynamically generated>")

        return workflow_func, config

    except Exception as e:
        print(f"❌ Error inspecting {yaml_file}: {e}")
        import traceback
        traceback.print_exc()
        return None, None

def main():
    """Main function to inspect all example workflows."""
    examples = [
        'test_basic.yml',
        'test_foreach.yml',
        'test_conditional.yml',
        'test_collect.yml',
        'test_complex.yml'
    ]

    print("🔬 YAML Workflow Code Inspection")
    print("=" * 60)

    for yaml_file in examples:
        if Path(yaml_file).exists():
            workflow_func, config = inspect_yaml_workflow(yaml_file)
        else:
            print(f"❌ File not found: {yaml_file}")

if __name__ == "__main__":
    main()