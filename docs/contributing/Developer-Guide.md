

# Developer Guide

## Python Environment Setup

Set up packages and linter tools on your development environment

```bash
$ cd $REPO_ROOT/python
$ poetry install -E dev
```

## Check before create commit

```bash
# under `$REPO_ROOT/python` directory
$ poetry run pre-commit
```

## Check in manual

```bash
# under `$REPO_ROOT/python` directory
# lint check
$ poetry run ruff check $PYTHON_FILE_NAME

# Run formatter
$ poetry run ruff format $PYTHON_FILE_NAME
```