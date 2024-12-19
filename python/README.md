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
- Run examples: `poetry run python -m examples.bert_cola`
- Format code: `poetry run black .`
- Run Michelangelo CLI: `ma --help`

### Environment Setup: Mac

- Install Python 3.9: `brew install python@3.9`
- Create virtual environment: `/usr/local/bin/python3.9 -m venv .venv`
- Activate virtual environment: `source .venv/bin/activate`
