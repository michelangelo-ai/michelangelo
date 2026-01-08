#!/usr/bin/env python3
"""Script to run YAML workflow examples and show outputs."""

import sys
import os
from pathlib import Path

# Add the current directory to path so we can import example_tasks
sys.path.insert(0, str(Path(__file__).parent))

def run_basic_example():
    """Run the basic single-task workflow."""
    print("🔧 Running Basic Workflow")
    print("=" * 50)

    # Override sys.argv to simulate command line
    original_argv = sys.argv.copy()
    sys.argv = ['run_yaml_examples.py', 'yaml-local-run']

    try:
        import michelangelo.uniflow as uniflow

        # Create context and run workflow
        ctx = uniflow.create_context()
        print(f"Execution mode: {'Local' if ctx.is_local_run() else 'Remote'}")
        print(f"YAML workflow: {ctx.is_yaml_workflow()}")
        print()

        result = ctx.run('test_basic.yml')

        print("📋 Workflow Results:")
        if isinstance(result, dict):
            for task_name, task_result in result.items():
                print(f"  {task_name}: {task_result}")
        else:
            print(f"  Result: {result}")

        return True

    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False
    finally:
        sys.argv = original_argv

def run_foreach_example():
    """Run the foreach/expand workflow."""
    print("\\n🔄 Running Foreach/Expand Workflow")
    print("=" * 50)

    # Override sys.argv to simulate command line
    original_argv = sys.argv.copy()
    sys.argv = ['run_yaml_examples.py', 'yaml-local-run']

    try:
        import michelangelo.uniflow as uniflow

        # Create context and run workflow
        ctx = uniflow.create_context()
        print(f"Execution mode: {'Local' if ctx.is_local_run() else 'Remote'}")
        print()

        result = ctx.run('test_foreach.yml')

        print("📋 Workflow Results:")
        if isinstance(result, dict):
            for task_name, task_result in result.items():
                print(f"  {task_name}: {type(task_result)} -> {task_result}")

        return True

    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False
    finally:
        sys.argv = original_argv

def run_collect_example():
    """Run the collect/aggregate workflow."""
    print("\\n📊 Running Collect/Aggregate Workflow")
    print("=" * 50)

    # Override sys.argv to simulate command line
    original_argv = sys.argv.copy()
    sys.argv = ['run_yaml_examples.py', 'yaml-local-run']

    try:
        import michelangelo.uniflow as uniflow

        # Create context and run workflow
        ctx = uniflow.create_context()
        print(f"Execution mode: {'Local' if ctx.is_local_run() else 'Remote'}")
        print()

        result = ctx.run('test_collect.yml')

        print("📋 Workflow Results:")
        if isinstance(result, dict):
            for task_name, task_result in result.items():
                if isinstance(task_result, list) and len(task_result) > 3:
                    print(f"  {task_name}: List with {len(task_result)} items")
                    print(f"    First item: {task_result[0]}")
                    print(f"    Last item: {task_result[-1]}")
                else:
                    print(f"  {task_name}: {task_result}")

        return True

    except Exception as e:
        print(f"❌ Error: {e}")
        import traceback
        traceback.print_exc()
        return False
    finally:
        sys.argv = original_argv

def main():
    """Run all examples and show results."""
    print("🧪 Running YAML Workflow Examples")
    print("=" * 60)

    examples = [
        ("Basic Workflow", run_basic_example),
        ("Foreach/Expand Workflow", run_foreach_example),
        ("Collect/Aggregate Workflow", run_collect_example),
    ]

    results = {}

    for name, func in examples:
        print(f"\\n🚀 Starting: {name}")
        try:
            success = func()
            results[name] = "✅ Success" if success else "❌ Failed"
        except Exception as e:
            results[name] = f"❌ Exception: {e}"
        print(f"Completed: {name}")

    # Summary
    print("\\n" + "=" * 60)
    print("📋 EXECUTION SUMMARY")
    print("=" * 60)
    for name, status in results.items():
        print(f"{name:25} {status}")

if __name__ == "__main__":
    main()