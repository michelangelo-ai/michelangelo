"""
External storage handler for pipeline manifest content.

This module implements external storage for JSON content to avoid protobuf conversion
and maintain readable JSON format in pipeline manifests.
"""

import json
import logging
import uuid
from typing import Dict, Any
import fsspec

_logger = logging.getLogger(__name__)


class ExternalStorageHandler:
    """Handler for storing and retrieving JSON content from external storage."""
    
    def __init__(self, base_url: str = "s3://default/pipeline-content"):
        """
        Initialize external storage handler.
        
        Args:
            base_url: Base URL for external storage (e.g., s3://bucket/path)
        """
        self.base_url = base_url.rstrip("/")
        
    def store_json_content(self, content: Dict[str, Any], content_id: str = None) -> str:
        """
        Store JSON content externally and return reference.
        
        Args:
            content: Dictionary content to store as JSON
            content_id: Optional content ID, generates UUID if not provided
            
        Returns:
            str: Reference URL to the stored content
            
        Raises:
            Exception: If storage operation fails
        """
        if content_id is None:
            content_id = str(uuid.uuid4())
            
        storage_path = f"{self.base_url}/{content_id}.json"
        
        try:
            _logger.info("Storing JSON content to external storage: %s", storage_path)
            
            # Serialize content to JSON
            json_str = json.dumps(content, indent=2)
            
            # Upload to external storage using fsspec
            fs, path = fsspec.core.url_to_fs(storage_path)
            with fs.open(path, "w") as f:
                f.write(json_str)
                
            _logger.info("Successfully stored content externally: %s", storage_path)
            _logger.debug("Stored content: %s", json_str)
            
            return storage_path
            
        except Exception as e:
            _logger.error("Failed to store content externally: %s", e)
            raise
    
    def fetch_json_content(self, reference: str) -> Dict[str, Any]:
        """
        Fetch JSON content from external storage.
        
        Args:
            reference: Reference URL to the stored content
            
        Returns:
            Dict[str, Any]: Retrieved JSON content
            
        Raises:
            Exception: If fetch operation fails
        """
        try:
            _logger.info("Fetching JSON content from external storage: %s", reference)
            
            # Fetch from external storage using fsspec
            fs, path = fsspec.core.url_to_fs(reference)
            with fs.open(path, "r") as f:
                content = json.load(f)
                
            _logger.info("Successfully fetched content from external storage")
            _logger.debug("Fetched content: %s", content)
            
            return content
            
        except Exception as e:
            _logger.error("Failed to fetch content from external storage: %s", e)
            raise
    
    def create_external_reference(self, storage_path: str) -> Dict[str, Any]:
        """
        Create an external reference structure for protobuf Any field.
        
        Args:
            storage_path: Path where content is stored
            
        Returns:
            Dict[str, Any]: External reference structure using google.protobuf.StringValue
        """
        return {
            "@type": "type.googleapis.com/google.protobuf.StringValue",
            "value": f"external:{storage_path}"
        }


# Global instance using default MinIO storage
default_external_storage = ExternalStorageHandler()