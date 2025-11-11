# test_generated_code.py
import sys
import os

# Ensure that the generated code is in the module search path
# Adjust the path to point to the correct location where the generated code is located
sys.path.insert(
    0, os.path.abspath(os.path.join(os.path.dirname(__file__), "michelangelo/gen"))
)

from michelangelo.gen.api.v2.project_pb2 import Project
from michelangelo.api.v2.client import APIClient

# Initialize the API client
APIClient.set_caller("test-client")

# List existing projects (replace with an actual namespace if needed)
try:
    projects = APIClient.ProjectService.list_project(namespace="default")
    print("Existing projects:")
    print(projects)
except Exception as e:
    print(f"Error listing projects: {e}")

# Create a new project
try:
    proj = Project()
    proj.metadata.namespace = "demo-project"
    proj.metadata.name = "demo-project"
    proj.spec.tier = 2
    proj.spec.description = "demo project"
    proj.spec.owner.owning_team = "8D8AC610-566D-4EF0-9C22-186B2A5ED793"
    proj.spec.git_repo = "https://github.com/michelangelo-ai/michelangelo"
    proj.spec.root_dir = "/demo-project"

    # Assuming the create_project method is correct
    APIClient.ProjectService.create_project(proj)
    print("Created project:", proj)
except Exception as e:
    print(f"Error creating project: {e}")

# Get the newly created project
try:
    project = APIClient.ProjectService.get_project(
        namespace="demo-project", name="demo-project"
    )
    print("Retrieved project:")
    print(project)
except Exception as e:
    print(f"Error retrieving project: {e}")
