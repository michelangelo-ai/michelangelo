"""Michelangelo API v2 Python client.

This package provides a Python client for interacting with the Michelangelo 2.0 API.
The main entry point is the APIClient class, which provides access to all API services.

Example:
    >>> from michelangelo.api.v2 import APIClient
    >>> APIClient.set_caller('my-application')
    >>> model = APIClient.ModelService.get_model(namespace='default', name='my-model')

The package also exposes ``generate_random_name`` for callers that need a unique,
time-sortable object name when they have no stable identity of their own:

    >>> from michelangelo.api.v2 import generate_random_name
    >>> generate_random_name('model')  # doctest: +SKIP
    'model-20260721-114130-2d9c959d'
"""

from .client import APIClient as APIClient
from .util import generate_random_name as generate_random_name
