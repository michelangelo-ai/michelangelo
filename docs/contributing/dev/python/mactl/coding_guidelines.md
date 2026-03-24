# Mactl Coding Guidelines

This document outlines coding best practices when developing features for the Michelangelo mactl CLI tool.

## Test File Organization

### Where to Place Tests

When adding features to mactl, follow this test file organization:

#### ✅ **Correct Test Locations**

```
python/michelangelo/cli/mactl/plugins/entity/{entity_type}/{feature}_test.py
```

**Examples:**
- Pipeline dev_run feature: `python/michelangelo/cli/mactl/plugins/entity/pipeline/dev_run_test.py`
- Pipeline create feature: `python/michelangelo/cli/mactl/plugins/entity/pipeline/create_test.py`
- Pipeline run feature: `python/michelangelo/cli/mactl/plugins/entity/pipeline/run_test.py`
- Pipeline apply feature: `python/michelangelo/cli/mactl/plugins/entity/pipeline/apply_test.py`
- Pipeline kill feature: `python/michelangelo/cli/mactl/plugins/entity/pipeline/kill_test.py`
- Trigger run entity: `python/michelangelo/cli/mactl/plugins/entity/trigger_run/main_test.py`
- Pipeline run entity: `python/michelangelo/cli/mactl/plugins/entity/pipeline_run/main_test.py`

#### ❌ **Avoid These Locations**

```
# Don't put mactl tests in generic test directories
python/tests/uniflow/core/test_{feature}.py        # Too generic
python/tests/mactl/test_{feature}.py                # Separated from code
python/michelangelo/cli/{feature}_test.py           # Wrong level
```

### Benefits of Co-located Tests

- **Discoverability**: Tests are next to the code they test
- **Maintainability**: Easier to find and update tests when changing features
- **Modularity**: Each plugin maintains its own test suite
- **Clear ownership**: Plugin developers own both code and tests

## Exception Handling Best Practices

### Use Context Managers Instead of try/finally

#### ✅ **Preferred: Use `with` statements for resource management**

```python
import tempfile
from pathlib import Path

def test_with_temporary_files():
    """Test using context manager for automatic cleanup."""
    with tempfile.TemporaryDirectory() as tmpdir:
        temp_file_path = Path(tmpdir) / "test_file.yaml"
        with open(temp_file_path, "w") as f:
            f.write(test_content)

        # Test logic here using temp_file_path
        result = process_yaml_file(temp_file_path)

        # No manual cleanup needed!
        # Files automatically removed when exiting the with block

    # temp_file_path no longer exists here
```

#### ❌ **Avoid: Manual try/finally cleanup**

```python
# Don't do this - prone to errors and verbose
def test_with_manual_cleanup():
    temp_file = None
    try:
        # Create temporary file
        with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
            f.write(test_content)
        temp_file = f.name

        # Test logic here
        result = process_yaml_file(temp_file)

    finally:
        # Manual cleanup - can fail or be forgotten!
        if temp_file and os.path.exists(temp_file):
            os.unlink(temp_file)
```

### Context Manager Benefits

1. **Automatic cleanup**: Resources cleaned up even if exceptions occur
2. **Exception safety**: Guaranteed cleanup on any exit path
3. **Cleaner code**: No need for try/finally blocks
4. **Less error-prone**: Can't forget to clean up resources
5. **More Pythonic**: Follows Python's resource management conventions

### Common Context Managers for mactl

```python
# File operations
with tempfile.TemporaryDirectory() as tmpdir:
    # Work with temporary directory

with open(file_path, "w") as f:
    # Write to file

# Mock/patch operations
with patch("module.function") as mock_func:
    # Test with mocked function

# Custom context managers
with kubernetes_cluster_context():
    # Test with K8s cluster

with mock_api_server():
    # Test with mock API responses
```

## File Handling Guidelines

### Temporary Files and Directories

#### ✅ **Use tempfile.TemporaryDirectory for test files**

```python
import tempfile
from pathlib import Path

def test_pipeline_creation():
    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = Path(tmpdir) / "pipeline.yaml"

        # Write test configuration
        with open(config_path, "w") as f:
            f.write("""
metadata:
  name: test-pipeline
  namespace: test-project
spec:
  steps: []
""")

        # Run test
        result = create_pipeline(config_path)
        assert result.success

        # Automatic cleanup when function exits
```

#### ✅ **Use pathlib.Path for file operations**

```python
from pathlib import Path

# Preferred - more readable and cross-platform
config_file = Path(tmpdir) / "config.yaml"
output_dir = Path(tmpdir) / "outputs"
output_dir.mkdir(parents=True, exist_ok=True)

# Instead of string concatenation
config_file = os.path.join(tmpdir, "config.yaml")  # Less readable
```

## Testing Best Practices

### Mock External Dependencies

```python
@patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.Repo")
@patch("michelangelo.cli.mactl.plugins.entity.pipeline.dev_run.generate_pipeline_run_name")
def test_pipeline_dev_run(self, mock_generate_name, mock_repo):
    """Test pipeline dev_run with mocked dependencies."""
    # Setup mocks
    mock_generate_name.return_value = "test-run-12345"
    mock_repo.return_value.git.rev_parse.return_value = "/fake/repo"

    # Test the function
    result = pipeline_dev_run(config_dict, crd_class, yaml_path)

    # Verify mocks were called correctly
    mock_generate_name.assert_called_once()
    mock_repo.assert_called_once()
```

### Test File Structure

```python
class PipelineDevRunTest(TestCase):
    """Tests for pipeline dev_run plugin."""

    def test_basic_functionality(self):
        """Test basic happy path."""
        pass

    def test_error_handling(self):
        """Test error conditions."""
        pass

    def test_edge_cases(self):
        """Test boundary conditions."""
        pass

    def test_integration_with_dependencies(self):
        """Test interaction with external systems."""
        pass
```

## Summary

1. **Test Location**: Place tests next to the code they test in the plugin structure
2. **Resource Management**: Use `with` statements instead of try/finally for cleanup
3. **File Handling**: Use `tempfile.TemporaryDirectory()` and `pathlib.Path`
4. **Exception Safety**: Let context managers handle cleanup automatically
5. **Maintainability**: Keep tests close to code for better organization

Following these guidelines will make your mactl features more robust, maintainable, and consistent with the rest of the codebase.