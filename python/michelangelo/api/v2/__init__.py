"""Michelangelo API v2 Python client.

This package provides a Python client for interacting with the Michelangelo 2.0 API.
The main entry point is the APIClient class, which provides access to all API services.

Example:
    >>> from michelangelo.api.v2 import APIClient
    >>> APIClient.set_caller('my-application')
    >>> model = APIClient.ModelService.get_model(namespace='default', name='my-model')
"""

from .client import APIClient
