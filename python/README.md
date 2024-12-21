Michelangelo SDK

## User Guide

```
pip install michelangelo
```

TODO: User Guide

## Developer Guide

### Preprequisites

- Python 3.9
- Poetry: https://python-poetry.org

### Cheat Sheet

- Install dependencies: `poetry install`
- Add a new dependency: `poetry add <package-name>`
- Run tests: `poetry run pytest`
- Run examples: `poetry run python ./examples/bert_cola/bert_cola.py`
- Format code: `poetry run black .`
- Run Michelangelo CLI: `poetry run ma --help`

### Environment Setup: Mac

- Install Python 3.9: `brew install python@3.9`
- Install Poetry: `curl -sSL https://install.python-poetry.org | python3.9 -`
- Create Python virtual environment and install packages: `poetry install`

The last step will create a `.venv` directory if it doesn't already exist.
This directory contains a Python virtual environment with all the dependencies installed.
You can activate this virtual environment and use it like any other Python virtual environment, 
or you can run commands via the Poetry CLI, e.g., `poetry run python`, `poetry run pytest`, etc.
