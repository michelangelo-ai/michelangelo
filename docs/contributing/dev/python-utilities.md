---
sidebar_position: 3
sidebar_label: "Python Utilities"
---

# Python Utilities and Scripts

## Scope

This page covers Python code in the repo that supports development workflows — tooling, linting, testing infrastructure. For the user-facing Uniflow SDK and `ma` CLI, see [Python Coding Guidelines](python/mactl/coding_guidelines.md).

## Package Manager

All Python work uses Poetry. The `pyproject.toml` lives in `python/`. To set up the full development environment:

```bash
cd python
poetry install -E dev
```

## Key Python Directories

| Directory | Contents |
|-----------|---------|
| `python/michelangelo/uniflow/` | Uniflow SDK (`@task`, `@workflow` decorators, task configs) |
| `python/michelangelo/cli/` | `ma` CLI tool |
| `python/michelangelo/uniflow/plugins/` | Python layer of each Uniflow plugin (RayTask, SparkTask, etc.) |

## Pre-commit Hooks

Pre-commit hooks are configured in `.pre-commit-config.yaml`. Hooks run ruff linting and formatting, trailing whitespace checks, and YAML validation. To run hooks manually before committing:

```bash
cd python
poetry run pre-commit
```

To install the hooks so they run automatically on `git commit`:

```bash
pip install pre-commit
pre-commit install
```

## Linting and Formatting

```bash
cd python
poetry run ruff check path/to/file.py   # lint
poetry run ruff format path/to/file.py  # format
```

Ruff configuration lives in `python/pyproject.toml`.

## Testing Python Code

```bash
cd python
poetry run pytest path/to/test_file.py
```

For running all tests:

```bash
cd python
poetry run pytest
```

## Dependencies

Add dependencies to `python/pyproject.toml`. Use extras (e.g., `-E ray`, `-E spark`) for optional dependencies that should not be required for all users. After changing `pyproject.toml`, regenerate the lockfile:

```bash
cd python
poetry lock
```

Check in both `pyproject.toml` and `poetry.lock` together.

## Related

- [Python Coding Guidelines](python/mactl/coding_guidelines.md)
- [Uniflow Plugin Guide](../uniflow-plugin-guide.md)
- [Testing Strategy](../testing.md)
- [YAML Configuration](yaml-configuration.md)
