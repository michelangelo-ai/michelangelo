"""
Configuration builder for uniflow pipeline registration.

This module provides the ConfigBuilder class for parsing workflow configurations,
extracting workflow functions, and serializing config data for pipeline manifests.
"""

import inspect
import json
import logging
import sys
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Callable, Dict, Optional

import yaml

from michelangelo.canvas.lib.shared.json_data import JSONData
from michelangelo.uniflow.core.utils import import_attribute

_logger = logging.getLogger(__name__)


class ConfigEncoder(json.JSONEncoder):
    """Custom JSON encoder for workflow configurations."""

    def default(self, obj: Any) -> Any:
        """Custom encoding for special objects."""
        if isinstance(obj, JSONData):
            return obj.model_dump(
                exclude_defaults=False, context={"UniflowCodec": True}
            )

        # Handle other common types that might need special encoding
        if hasattr(obj, "model_dump"):
            return obj.model_dump(exclude_defaults=False)

        if hasattr(obj, "__dataclass_fields__"):
            return {
                field.name: getattr(obj, field.name)
                for field in obj.__dataclass_fields__.values()
            }

        if hasattr(obj, "to_dict"):
            return obj.to_dict()

        if hasattr(obj, "tolist"):
            return obj.tolist()

        return super().default(obj)


class ConfigBuilder:
    """Builder for workflow configuration and metadata extraction."""

    def __init__(
        self, workflow_function_obj: Callable, config_data: Optional[Dict] = None
    ):
        """
        Initialize ConfigBuilder with workflow function and optional config.

        Args:
            workflow_function_obj: The workflow function object
            config_data: Optional configuration data from YAML
        """
        self._workflow_function_obj = workflow_function_obj
        self._config_data = config_data or {}
        self._workflow_config = self._build_workflow_config()

    @classmethod
    @contextmanager
    def from_config_file(cls, config_file_path: str):
        """
        Create ConfigBuilder from configuration file.

        Args:
            config_file_path: Path to the pipeline YAML configuration file

        Yields:
            ConfigBuilder: Configured builder instance

        Raises:
            ValueError: If no workflow function found or multiple found
            ImportError: If module cannot be imported
        """
        _logger.info("Creating ConfigBuilder from config file: %s", config_file_path)

        # Read YAML configuration
        with open(config_file_path, "r") as f:
            config = yaml.safe_load(f)

        # Extract manifest path
        manifest_data = config.get("spec", {}).get("manifest", {})
        manifest_path = manifest_data.get("filePath") or manifest_data.get("path")

        if not manifest_path:
            raise ValueError(
                f"No manifest path found in config file: {config_file_path}"
            )

        _logger.info("Found manifest path: %s", manifest_path)

        # Import and discover workflow function
        workflow_function_obj = cls._discover_workflow_function(manifest_path)

        try:
            builder = cls(workflow_function_obj, config)
            yield builder
        finally:
            # Cleanup if needed
            pass

    @staticmethod
    def _discover_workflow_function(manifest_path: str) -> Callable:
        """
        Discover workflow function from manifest path.

        Args:
            manifest_path: Module path like "module.submodule:function" or "module.submodule"

        Returns:
            Callable: The discovered workflow function

        Raises:
            ValueError: If workflow function not found
            ImportError: If module cannot be imported
        """
        _logger.info("Discovering workflow function from path: %s", manifest_path)

        # Handle different path formats
        if ":" in manifest_path:
            module_path, function_name = manifest_path.split(":", 1)
            try:
                # Import the specific function
                workflow_function = import_attribute(f"{module_path}.{function_name}")
                _logger.info("Successfully imported workflow function: %s", function_name)
                return workflow_function
            except (ImportError, AttributeError) as e:
                raise ImportError(
                    f"Could not import workflow function {function_name} from {module_path}: {e}"
                )
        else:
            # No function specified, import module and find workflow function
            module_path = manifest_path
            try:
                # Import the module directly (like subprocess.py does)
                import importlib
                module = importlib.import_module(module_path)
                
                # Find workflow-decorated functions in the module
                workflow_functions = []
                for attr_name in dir(module):
                    attr = getattr(module, attr_name)
                    if callable(attr) and hasattr(attr, '__name__'):
                        # Check if it's decorated with @workflow (look for workflow decorator metadata)
                        if hasattr(attr, '_workflow_config') or 'workflow' in attr_name.lower():
                            workflow_functions.append((attr_name, attr))
                
                if len(workflow_functions) == 0:
                    raise ValueError(f"No workflow function found in module {module_path}")
                elif len(workflow_functions) == 1:
                    function_name, workflow_function = workflow_functions[0]
                    _logger.info("Successfully found workflow function: %s", function_name)
                    return workflow_function
                else:
                    # Multiple workflow functions found, try to pick the most likely one
                    function_names = [name for name, _ in workflow_functions]
                    _logger.warning("Multiple workflow functions found: %s", function_names)
                    # Pick the first one that contains 'workflow' in name
                    for name, func in workflow_functions:
                        if 'workflow' in name:
                            _logger.info("Selected workflow function: %s", name)
                            return func
                    # Fallback to first one
                    function_name, workflow_function = workflow_functions[0]
                    _logger.info("Selected first workflow function: %s", function_name)
                    return workflow_function
                    
            except (ImportError, AttributeError) as e:
                raise ImportError(f"Could not import module {module_path}: {e}")

    def _build_workflow_config(self) -> Dict[str, Any]:
        """Build workflow configuration from function and config data."""
        # Get function signature
        sig = inspect.signature(self._workflow_function_obj)

        # Extract parameters and build config
        workflow_config = {
            "function_name": self._workflow_function_obj.__name__,
            "module_name": self._workflow_function_obj.__module__,
            "parameters": {},
            "task_configs": {},
        }

        # Add function parameters
        for param_name, param in sig.parameters.items():
            default_value = (
                param.default if param.default != inspect.Parameter.empty else None
            )
            workflow_config["parameters"][param_name] = {
                "name": param_name,
                "type": str(param.annotation)
                if param.annotation != inspect.Parameter.empty
                else "Any",
                "default": default_value,
            }

        # Add any additional config from YAML
        if self._config_data:
            workflow_config.update(self._config_data.get("workflow_config", {}))

        return workflow_config

    def get_workflow_func_with_task_override(self) -> Callable:
        """Get workflow function with task overrides applied."""
        # For now, return the original function
        # In a complete implementation, this would apply task overrides
        return self._workflow_function_obj

    def get_workflow_config_in_json(self) -> str:
        """
        Serialize workflow config to JSON format.

        Returns:
            str: JSON string representation of workflow config
        """
        _logger.info("Serializing workflow config to JSON")

        # Use custom encoder for special objects
        json_str = json.dumps(self._workflow_config, cls=ConfigEncoder, indent=2)

        # Parse and re-serialize to ensure clean JSON
        json_obj = json.loads(json_str)
        return json.dumps(json_obj)

    @property
    def workflow_function(self) -> str:
        """Get the workflow function name as string."""
        return f"{self._workflow_function_obj.__module__}.{self._workflow_function_obj.__name__}"

    @property
    def workflow_function_obj(self) -> Callable:
        """Get the workflow function object."""
        return self._workflow_function_obj

    def get_workflow_args(self) -> list:
        """
        Extract positional arguments for the workflow function.
        
        Returns:
            list: List of positional argument values (empty for workflows using kwargs)
        """
        # Workflows typically use keyword arguments, so return empty list
        # This allows for proper JSON structure in manifest
        return []

    def get_workflow_kwargs(self) -> dict:
        """
        Extract keyword arguments for the workflow function.
        
        First tries to extract from ctx.run() calls in the module,
        then falls back to default parameter values.
        
        Returns:
            dict: Dictionary of parameter names and their values
        """
        sig = inspect.signature(self._workflow_function_obj)
        kwargs = {}
        
        # First, try to extract kwargs from ctx.run() calls in the module
        try:
            module = import_attribute(self._workflow_function_obj.__module__)
            
            # Read the module source to extract ctx.run calls
            import ast
            import os
            
            # Get the module file path
            module_file = module.__file__
            if module_file and os.path.exists(module_file):
                with open(module_file, 'r') as f:
                    source = f.read()
                
                # Parse the AST to find ctx.run calls
                tree = ast.parse(source)
                
                for node in ast.walk(tree):
                    # Look for calls like: ctx.run(train_workflow, param=value)
                    if (isinstance(node, ast.Call) and
                        isinstance(node.func, ast.Attribute) and
                        isinstance(node.func.value, ast.Name) and
                        node.func.value.id == 'ctx' and
                        node.func.attr == 'run'):
                        
                        # Check if this is calling our workflow function
                        if (len(node.args) > 0 and
                            isinstance(node.args[0], ast.Name) and
                            node.args[0].id == self._workflow_function_obj.__name__):
                            
                            # Extract keyword arguments from the ctx.run call
                            for keyword in node.keywords:
                                if isinstance(keyword.value, ast.Constant):
                                    kwargs[keyword.arg] = keyword.value.value
                                elif isinstance(keyword.value, ast.Str):  # Python < 3.8
                                    kwargs[keyword.arg] = keyword.value.s
                            break
                        
        except Exception as e:
            _logger.warning("Could not extract kwargs from ctx.run calls: %s", e)
        
        # Fall back to extracting parameters with default values
        for param_name, param in sig.parameters.items():
            if param_name not in kwargs and param.default != inspect.Parameter.empty:
                kwargs[param_name] = param.default
        
        _logger.info("Extracted workflow kwargs: %s", kwargs)
        return kwargs

    def get_workflow_environ(self) -> dict:
        """
        Get workflow environment variables by analyzing the workflow module.
        
        Returns:
            dict: Environment variables for the workflow
        """
        environ = {}
        
        # Try to extract environment variables from the workflow module
        try:
            module = import_attribute(self._workflow_function_obj.__module__)
            
            # Read the module source to extract ctx.environ assignments
            import ast
            import os
            
            # Get the module file path
            module_file = module.__file__
            if module_file and os.path.exists(module_file):
                with open(module_file, 'r') as f:
                    source = f.read()
                
                # Parse the AST to find ctx.environ assignments
                tree = ast.parse(source)
                
                for node in ast.walk(tree):
                    # Look for assignments like: ctx.environ["KEY"] = "value"
                    if (isinstance(node, ast.Assign) and 
                        len(node.targets) == 1 and
                        isinstance(node.targets[0], ast.Subscript) and
                        isinstance(node.targets[0].value, ast.Attribute) and
                        isinstance(node.targets[0].value.value, ast.Name) and
                        node.targets[0].value.value.id == 'ctx' and
                        node.targets[0].value.attr == 'environ'):
                        
                        # Extract key and value
                        if isinstance(node.targets[0].slice, ast.Constant):
                            key = node.targets[0].slice.value
                        elif isinstance(node.targets[0].slice, ast.Str):  # Python < 3.8
                            key = node.targets[0].slice.s
                        else:
                            continue
                            
                        if isinstance(node.value, ast.Constant):
                            value = node.value.value
                        elif isinstance(node.value, ast.Str):  # Python < 3.8
                            value = node.value.s
                        else:
                            continue
                            
                        environ[key] = str(value)
                        
        except Exception as e:
            _logger.warning("Could not extract environment variables from workflow: %s", e)
        
        # Extract environment variables from config data if available
        if self._config_data and "environ" in self._config_data:
            environ.update(self._config_data["environ"])
            
        _logger.info("Extracted workflow environ: %s", environ)
        return environ

    def get_workflow_config_as_manifest_content(self) -> dict:
        """
        Get workflow configuration formatted for manifest content.
        
        Returns:
            dict: Configuration in the format expected by manifest content
        """
        return {
            "args": self.get_workflow_args(),
            "kwargs": [[k, v] for k, v in self.get_workflow_kwargs().items()],
            "environ": self.get_workflow_environ()
        }
