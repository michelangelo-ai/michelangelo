#!/usr/bin/env python3
"""
Demo environment for mactl command line tool.
Provides mock responses with realistic outputs and UI URLs for demonstration purposes.
"""

import sys
import json
import time
import random
from datetime import datetime
from pathlib import Path

# =============================================================================
# ALLOWED COMMANDS CONFIGURATION
# =============================================================================
# Add or modify commands here to control what the demo supports

ALLOWED_COMMANDS = {
    # Project commands
    "apply -f project.yaml": {
        "response": "✅ Project created successfully!\n📄 Resource: demo-ml-project\n🆔 ID: res-1234\n📍 Namespace: ml-demo\n🏠 Project Home: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test-uber-one/"
    },
    "project list": {
        "response": "dynamic_project_list"  # Special marker for dynamic responses
    },
    "project get nlp-models": {
        "response": "dynamic_project_get"
    },

    # Pipeline commands
    "pipeline list": {
        "response": "dynamic_pipeline_list"
    },
    "pipeline run": {
        "response": "dynamic_pipeline_run"
    },
    "pipeline run -n ma-dev-test-uber-one --revision pipeline-simple-custom-train-3f4c8655edc4": {
        "response": "Status Succeeded: Successfully created a new pipelineRun run-20251001-101108-c5615c76\nMA Studio URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test-uber-one/train/runs/run-20250930-001156-09c7468f"
    },
    "pipeline run -n ma-dev-test-uber-one --revision pipeline-simple-custom-train-3f4c8655edc4 --resume_from run-20251001-013007-f22d76f3:tabular_trainer": {
        "response": "Status Succeeded: Successfully created a new pipelineRun run-20251001-101108-ca2ec908\nMA Studio URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test-uber-one/train/runs/run-20251001-101108-ee41b395"
    },
    "pipeline run -n ma-dev-test-uber-one --revision pipeline-boston-housing-keras-ig-explainer-eval-26d199b842a7": {
        "response": "Status Succeeded: Successfully created a new pipelineRun run-20250912-181601-e371aba4\nMA Studio URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test/train/runs/run-20250912-181601-e371aba4"
    },
    "pipeline run -n ma-dev-test-uber-one --revision pipeline-simple-custom-train-08ccb706e067": {
        "response": "Status Succeeded: Successfully created a new pipelineRun run-20251002-163458-b7d0a0be\nMA Studio URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test-uber-one/train/runs/run-20251002-163458-b7d0a0be"
    },
    "pipeline run -n ma-dev-test --revision pipeline-boston-housing-retrain-sdarre-612bc83ab80e": {
        "response": "Status Succeeded: Successfully created a new pipelineRun run-20250212-110233-0f1ea03b\nMA Studio URL: https://michelangelo-studio.uberinternal.com/ma/ma-customer-sandbox/retrain/runs/run-20250212-110233-0f1ea03b/steps"
    },
    "pipeline apply -f ./examples/pytorch_boston_housing/pipeline.yaml": {
        "response": "WARNING: Ignoring JAVA_HOME, because it must point to a JDK, not a JRE.\nINFO: Invocation ID: f050b7d3-d30a-4a10-a5a7-e9b10e0621fd\nWARNING: /home/user/uber-one/uber/ai/michelangelo/sdk/workflow/framework/container/layers/BUILD.bazel:3:4: in _write_file rule //uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl: target '//uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl' depends on deprecated target '@@bazel_tools//src/conditions:host_windows_x64_constraint': No longer used by Bazel and will be removed in the future. Migrate to toolchains or define your own version of this setting.\nINFO: Analyzed 4 targets (12 packages loaded, 109 targets configured).\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_feature_prep/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_trainer/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_assembler/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: Found 4 targets...\nINFO: Elapsed time: 18.626s, Critical Path: 16.67s\nINFO: 41 processes: 34 action cache hit, 16 internal, 25 processwrapper-sandbox.\nINFO: Build completed successfully, 41 total actions\nStatus Succeeded: Successfully updated the existing pipeline llm-train\nRevisionId:c20f1584b7247f2ec77fe014e7901e61ba7681af\n🌐 Pipeline URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test/train/pipelines/torch-maker-boston-explainer?revisionId=8a39d71cf00bba945829adf2964d708f7e813219"
    },
    "pipeline apply -f ./examples/pytorch_boston_housing_eval/pipeline.yaml": {
        "response": "WARNING: Ignoring JAVA_HOME, because it must point to a JDK, not a JRE.\nINFO: Invocation ID: f050b7d3-d30a-4a10-a5a7-e9b10e0621fd\nWARNING: /home/user/uber-one/uber/ai/michelangelo/sdk/workflow/framework/container/layers/BUILD.bazel:3:4: in _write_file rule //uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl: target '//uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl' depends on deprecated target '@@bazel_tools//src/conditions:host_windows_x64_constraint': No longer used by Bazel and will be removed in the future. Migrate to toolchains or define your own version of this setting.\nINFO: Analyzed 4 targets (12 packages loaded, 109 targets configured).\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_feature_prep/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_trainer/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_assembler/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: Found 4 targets...\nINFO: Elapsed time: 18.626s, Critical Path: 16.67s\nINFO: 41 processes: 34 action cache hit, 16 internal, 25 processwrapper-sandbox.\nINFO: Build completed successfully, 41 total actions\nStatus Succeeded: Successfully updated the existing pipeline bert-cola-eval\nRevisionId:26d199b842a7183f000bfdfa9c0d8bbdcc4f928b\n🌐 Pipeline URL: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test/train/pipelines/boston-housing-keras-ig-explainer-eval?revisionId=26d199b842a7183f000bfdfa9c0d8bbdcc4f928b"
    },
    "pipeline apply -f ./examples/retrain/pipeline.yaml": {
        "response": "WARNING: Ignoring JAVA_HOME, because it must point to a JDK, not a JRE.\nINFO: Invocation ID: f050b7d3-d30a-4a10-a5a7-e9b10e0621fd\nWARNING: /home/user/uber-one/uber/ai/michelangelo/sdk/workflow/framework/container/layers/BUILD.bazel:3:4: in _write_file rule //uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl: target '//uber/ai/michelangelo/sdk/workflow/framework/container/layers:workdir_mtree_tmpl' depends on deprecated target '@@bazel_tools//src/conditions:host_windows_x64_constraint': No longer used by Bazel and will be removed in the future. Migrate to toolchains or define your own version of this setting.\nINFO: Analyzed 4 targets (12 packages loaded, 109 targets configured).\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_feature_prep/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_trainer/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: From Writing: bazel-out/k8-fastbuild/bin/uber/ai/michelangelo/sdk/workflow/tasks/llm_assembler/canvas_default_task_39_py_image_layers_flattened.tar:\nDuplicate file in archive: home/udocker/app/, picking first occurrence\nINFO: Found 4 targets...\nINFO: Elapsed time: 18.626s, Critical Path: 16.67s\nINFO: 41 processes: 34 action cache hit, 16 internal, 25 processwrapper-sandbox.\nINFO: Build completed successfully, 41 total actions\nStatus Succeeded: Successfully updated the existing pipeline boston-housing-retrain\nRevisionId:6bd82e326c2545cb82593d30b1b1331e\n🌐 Pipeline URL: https://michelangelo-studio.uberinternal.com/ma/ma-customer-sandbox/retrain/pipelines/boston-housing-retrain?revisionId=6bd82e326c2545cb82593d30b1b1331e"
    },

    # Trigger commands
    "trigger apply -f trigger.yaml": {
        "response": "Status Succeeded: Successfully updated the existing trigger\n🌐 Pipeline with trigger: https://michelangelo-studio.uberinternal.com/ma/ma-dev-test-uber-one/train/pipelines/maf-train-simplified/triggers?revisionId=96f97a4ba507f175f8cc3c827c8ad01eeee83135"
    },

    # Deployment commands
    "deployment list": {
        "response": "dynamic_deployment_list"
    },
    "deployment apply deployment.yaml": {
        "response": "dynamic_deployment_apply"
    },

    # Help command
    "help": {
        "response": "show_help"
    },
    "-h": {
        "response": "show_help"
    },
    "--help": {
        "response": "show_help"
    }
}

# Demo configuration
DEMO_UI_BASE_URL = "https://michelangelo-studio.uberinternal.com/ma"
DEMO_RESOURCES = {
    "pipelines": [
        {"name": "bert-training-pipeline", "namespace": "ml-demo", "status": "Running"},
        {"name": "model-evaluation-pipeline", "namespace": "ml-demo", "status": "Completed"},
        {"name": "data-preprocessing-pipeline", "namespace": "ml-demo", "status": "Pending"}
    ],
    "deployments": [
        {"name": "bert-cola-deployment", "namespace": "ml-demo", "model": "bert-cola-32", "status": "Active"},
        {"name": "sentiment-model-deployment", "namespace": "ml-demo", "model": "sentiment-v2", "status": "Active"}
    ],
    "projects": [
        {"name": "nlp-models", "namespace": "ml-demo", "description": "NLP model development project", "status": "Active"},
        {"name": "computer-vision", "namespace": "ml-demo", "description": "Computer vision models", "status": "Active"}
    ],
    "inference_servers": [
        {"name": "inference-server-bert-cola", "namespace": "ml-demo", "status": "Running", "replicas": 2}
    ]
}

def generate_timestamp():
    """Generate a timestamp for demo responses."""
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")

def generate_resource_id():
    """Generate a random resource ID for demo purposes."""
    return f"res-{random.randint(1000, 9999)}"

def mock_pipeline_apply(file_path):
    """Mock pipeline apply command."""
    pipeline_name = f"demo-pipeline-{random.randint(100, 999)}"
    resource_id = generate_resource_id()

    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": "Pipeline",
        "metadata": {
            "name": pipeline_name,
            "namespace": "ml-demo",
            "uid": resource_id,
            "creationTimestamp": generate_timestamp(),
            "generation": 1
        },
        "spec": {
            "description": "Demo pipeline created from YAML",
            "status": "Created"
        },
        "status": {
            "phase": "Pending",
            "message": "Pipeline created successfully"
        }
    }

    ui_url = f"{DEMO_UI_BASE_URL}/pipelines/{pipeline_name}?namespace=ml-demo"

    print(f"✅ Pipeline created successfully!")
    print(f"📄 Resource: {pipeline_name}")
    print(f"🆔 ID: {resource_id}")
    print(f"📍 Namespace: ml-demo")
    print(f"🌐 View in UI: {ui_url}")
    print(f"\n📊 Full Response:")
    print(json.dumps(response, indent=2))

    return response

def mock_pipeline_run():
    """Mock pipeline run command."""
    run_id = f"run-{random.randint(10000, 99999)}"
    pipeline_name = random.choice(DEMO_RESOURCES["pipelines"])["name"]

    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": "PipelineRun",
        "metadata": {
            "name": f"{pipeline_name}-{run_id}",
            "namespace": "ml-demo",
            "uid": generate_resource_id(),
            "creationTimestamp": generate_timestamp()
        },
        "spec": {
            "pipelineName": pipeline_name,
            "parameters": {
                "batch_size": "32",
                "learning_rate": "0.001",
                "epochs": "10"
            }
        },
        "status": {
            "phase": "Running",
            "startTime": generate_timestamp(),
            "message": "Pipeline execution started"
        }
    }

    ui_url = f"{DEMO_UI_BASE_URL}/pipeline-runs/{run_id}?namespace=ml-demo"

    print(f"🚀 Pipeline run started!")
    print(f"📄 Run ID: {run_id}")
    print(f"🔗 Pipeline: {pipeline_name}")
    print(f"📍 Namespace: ml-demo")
    print(f"⏱️  Status: Running")
    print(f"🌐 Monitor in UI: {ui_url}")
    print(f"\n📊 Full Response:")
    print(json.dumps(response, indent=2))

    return response

def mock_deployment_apply(file_path):
    """Mock deployment apply command."""
    deployment_name = f"demo-deployment-{random.randint(100, 999)}"
    resource_id = generate_resource_id()

    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": "Deployment",
        "metadata": {
            "name": deployment_name,
            "namespace": "ml-demo",
            "uid": resource_id,
            "creationTimestamp": generate_timestamp(),
            "generation": 1
        },
        "spec": {
            "modelName": "bert-cola-v1",
            "replicas": 2,
            "resources": {
                "cpu": "1",
                "memory": "2Gi",
                "gpu": "1"
            }
        },
        "status": {
            "phase": "Deploying",
            "readyReplicas": 0,
            "totalReplicas": 2,
            "message": "Deployment in progress"
        }
    }

    ui_url = f"{DEMO_UI_BASE_URL}/deployments/{deployment_name}?namespace=ml-demo"
    inference_url = f"http://localhost:8080/inference-server-{deployment_name}-endpoint/{deployment_name}"

    print(f"🚀 Deployment created successfully!")
    print(f"📄 Resource: {deployment_name}")
    print(f"🆔 ID: {resource_id}")
    print(f"📍 Namespace: ml-demo")
    print(f"🔄 Status: Deploying (0/2 replicas ready)")
    print(f"🌐 View in UI: {ui_url}")
    print(f"🔗 Inference Endpoint: {inference_url}")
    print(f"\n📊 Full Response:")
    print(json.dumps(response, indent=2))

    return response

def mock_list_command(resource_type):
    """Mock list command for various resources."""
    resources = DEMO_RESOURCES.get(resource_type, [])

    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": f"{resource_type.capitalize()}List",
        "metadata": {
            "resourceVersion": "12345"
        },
        "items": []
    }

    print(f"📋 Listing {resource_type}:")
    print(f"{'NAME':<30} {'NAMESPACE':<15} {'STATUS':<15} {'AGE':<10}")
    print("-" * 70)

    for resource in resources:
        age = f"{random.randint(1, 30)}d"
        print(f"{resource['name']:<30} {resource['namespace']:<15} {resource['status']:<15} {age:<10}")

        item = {
            "metadata": {
                "name": resource['name'],
                "namespace": resource['namespace'],
                "uid": generate_resource_id(),
                "creationTimestamp": generate_timestamp()
            },
            "status": {
                "phase": resource['status']
            }
        }
        response["items"].append(item)

    ui_url = f"{DEMO_UI_BASE_URL}"
    print(f"\n🌐 View all {resource_type} in UI: {ui_url}")

    return response

def mock_get_command(resource_type, resource_name):
    """Mock get command for a specific resource."""
    resources = DEMO_RESOURCES.get(resource_type, [])
    resource = next((r for r in resources if r['name'] == resource_name), None)

    if not resource:
        print(f"❌ Error: {resource_type.capitalize()} '{resource_name}' not found")
        return None

    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": resource_type.capitalize().rstrip('s'),
        "metadata": {
            "name": resource['name'],
            "namespace": resource['namespace'],
            "uid": generate_resource_id(),
            "creationTimestamp": generate_timestamp(),
            "generation": random.randint(1, 5)
        },
        "spec": resource,
        "status": {
            "phase": resource['status'],
            "conditions": [
                {
                    "type": "Ready",
                    "status": "True" if resource['status'] in ["Active", "Running", "Completed"] else "False",
                    "lastTransitionTime": generate_timestamp()
                }
            ]
        }
    }

    ui_url = f"{DEMO_UI_BASE_URL}/{resource_type}/{resource_name}?namespace={resource['namespace']}"

    print(f"📄 {resource_type.capitalize()} Details:")
    print(f"  Name: {resource['name']}")
    print(f"  Namespace: {resource['namespace']}")
    print(f"  Status: {resource['status']}")
    print(f"  Created: {response['metadata']['creationTimestamp']}")
    print(f"🌐 View in UI: {ui_url}")
    print(f"\n📊 Full Response:")
    print(json.dumps(response, indent=2))

    return response

def mock_delete_command(resource_type, resource_name):
    """Mock delete command."""
    response = {
        "apiVersion": "michelangelo.api/v2beta1",
        "kind": resource_type.capitalize().rstrip('s'),
        "metadata": {
            "name": resource_name,
            "namespace": "ml-demo",
            "deletionTimestamp": generate_timestamp()
        },
        "status": {
            "phase": "Terminating"
        }
    }

    print(f"🗑️  {resource_type.capitalize()} '{resource_name}' deleted successfully!")
    print(f"📍 Namespace: ml-demo")
    print(f"⏱️  Status: Terminating")
    print(f"\n📊 Response:")
    print(json.dumps(response, indent=2))

    return response

def show_help():
    """Show help message with available commands."""
    help_text = """
🤖 Michelangelo AI Demo CLI (mactl)

Available Commands:
  mactl pipeline apply <file>     - Create/update a pipeline from YAML
  mactl pipeline run              - Start a pipeline run
  mactl pipeline list             - List all pipelines
  mactl pipeline get <name>       - Get pipeline details
  mactl pipeline delete <name>    - Delete a pipeline

  mactl deployment apply <file>   - Create/update a deployment from YAML
  mactl deployment list           - List all deployments
  mactl deployment get <name>     - Get deployment details
  mactl deployment delete <name>  - Delete a deployment

  mactl project apply <file>      - Create/update a project from YAML
  mactl project list              - List all projects
  mactl project get <name>        - Get project details

  mactl inference-server list     - List all inference servers
  mactl inference-server get <name> - Get inference server details

🌐 Demo UI Base URL: {DEMO_UI_BASE_URL}

Examples:
  mactl pipeline apply ./demo/training-pipeline.yaml
  mactl pipeline run
  mactl deployment list
  mactl deployment get bert-cola-deployment

All commands return realistic mock responses with UI URLs for demonstration.
"""
    print(help_text)

def main():
    """Main demo CLI function."""
    if len(sys.argv) < 2:
        show_help()
        return

    args = sys.argv[1:]
    command_str = " ".join(args)

    # Simulate some processing time
    print("🔄 Processing command...")
    time.sleep(0.5)

    # Check if the command is in ALLOWED_COMMANDS
    if command_str in ALLOWED_COMMANDS:
        response_type = ALLOWED_COMMANDS[command_str]["response"]

        if response_type == "show_help":
            show_help()
        elif response_type == "dynamic_project_list":
            mock_list_command("projects")
        elif response_type == "dynamic_project_get":
            mock_get_command("projects", "nlp-models")
        elif response_type == "dynamic_pipeline_list":
            mock_list_command("pipelines")
        elif response_type == "dynamic_pipeline_run":
            mock_pipeline_run()
        elif response_type == "dynamic_pipeline_apply":
            mock_pipeline_apply("training-pipeline.yaml")
        elif response_type == "dynamic_deployment_list":
            mock_list_command("deployments")
        elif response_type == "dynamic_deployment_apply":
            mock_deployment_apply("deployment.yaml")
        else:
            # For simple string responses
            print(response_type)
        return

    # If command not found, show error
    print(f"❌ Unknown command: {command_str}")
    print("\n📋 Available commands:")
    for cmd in sorted(ALLOWED_COMMANDS.keys()):
        print(f"  mactl {cmd}")
    print("\nRun 'mactl help' for more details")

if __name__ == "__main__":
    main()