---
name: Google Python Style Guide (PEP 8 + Google Extensions)
description: "Apply Python best practices, conventions, and style rules from Google's Python Style Guide. Use when writing, reviewing, or refactoring Python code to ensure clean, maintainable, and Pythonic implementations."
---

# Google Python Style Guide

Apply best practices and conventions from the official [Google Python Style Guide](https://google.github.io/styleguide/pyguide.html) to write clean, idiomatic Python code.

## When to Apply

Use this skill automatically when:
- Writing new Python code
- Reviewing Python code
- Refactoring existing Python implementations

## Important: PEP 8 Takes Precedence

**When Google's style guide conflicts with PEP 8, always follow PEP 8.** PEP 8 is the official Python style guide and is the canonical source for Python coding conventions. Google's guide extends and clarifies PEP 8 but should not override it.

Common areas where conflicts may occur:
- Line length (PEP 8: 79 chars, Google: 80-100 chars) → **Use Google style** (project standard)

## Key Reminders

Follow the conventions and patterns documented at https://google.github.io/styleguide/pyguide.html, with particular attention to:

### Imports
- Use `import` statements for packages and modules only
- Use `from x import y` where `x` is the package prefix and `y` is the module name with no prefix.
- Import each module using the full pathname location
- Avoid `from module import *`
- Order: standard library, third-party, local application imports

### Naming Conventions
- `module_name`, `package_name`, `ClassName`, `method_name`, `ExceptionName`
- `function_name`, `GLOBAL_CONSTANT_NAME`, `global_var_name`, `instance_var_name`
- `function_parameter_name`, `local_var_name`
- Single leading underscore `_` for internal use
- Avoid single letter names except for counters/iterators

### Documentation
- Use docstrings for all public modules, functions, classes, and methods
- Follow Google docstring format with Args, Returns, Raises sections
- First line should be a one-line summary
- Example:
  ```python
  def fetch_data(source: str, limit: int = 100) -> list[dict]:
      """Fetches data from the specified source.

      Args:
          source: The data source URL or path.
          limit: Maximum number of records to fetch.

      Returns:
          A list of dictionaries containing the fetched data.

      Raises:
          ValueError: If source is empty or invalid.
      """
  ```

### Type Annotations
- Use type hints for function signatures (Python 3.9+)
- Annotate public APIs
- Use `typing` module for complex types
- Prefer specific types over `Any`

### Error Handling
- Use specific exceptions, not bare `except:`
- Minimize code in `try/except` blocks
- Use `finally` for cleanup actions
- Prefer standard exceptions over custom ones

### Code Organization
- Maximum line length: 100 characters (Google standard, per project configuration)
- Use 4 spaces for indentation (never tabs)
- Two blank lines between top-level definitions
- One blank line between method definitions

### Best Practices
- **Comprehensions**: Use for simple cases; avoid complex nested comprehensions
- **Generators**: Prefer generators over lists for large datasets
- **Lambda**: Use only for one-liners; use `def` for anything more complex
- **Default Arguments**: Never use mutable objects as default values
- **Properties**: Use `@property` for accessing/setting attributes with logic
- **Context Managers**: Use `with` statements for resource management
- **String Formatting**: Prefer f-strings (Python 3.6+) or `str.format()`

### Anti-Patterns to Avoid
- Global variables (except constants)
- Mutable default arguments: `def foo(x=[]):` ❌
- Wildcard imports: `from module import *` ❌
- Bare except clauses: `except:` ❌
- Using deprecated features (e.g., `%` string formatting)

## References

- Official Guide: https://google.github.io/styleguide/pyguide.html
- PEP 8: https://peps.python.org/pep-0008/
- Python Documentation: https://docs.python.org/3/
