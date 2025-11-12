"""
Source definitions for Amazon Books Chronon features
Provides event sources for different aspects of the recommendation system
"""

from ai.chronon.api.ttypes import Source, EventSource
from ai.chronon.query import Query, select
from ai.chronon.utils import get_staging_query_output_table_name
from examples.amazon_books_qwen.data.staging_queries.amazon_books.books_reviews import (
    base_table,
)


def books_source(*columns):
    """
    Source for book-centric features based on reviews and metadata
    Used for building book popularity, rating trends, and content features
    """
    return Source(
        events=EventSource(
            table=get_staging_query_output_table_name(base_table),
            query=Query(
                selects=select(*columns),
                time_column="ts",
            ),
        )
    )


def user_interaction_source(*columns):
    """
    Source for user interaction events (reviews, ratings)
    Used for building user behavior patterns and preferences
    """
    return Source(
        events=EventSource(
            table=get_staging_query_output_table_name(base_table),
            query=Query(
                selects=select(*columns),
                time_column="ts",
            ),
        )
    )


def content_source(*columns):
    """
    Source for content-based features (book descriptions, categories)
    Used for text-based recommendation features
    """
    return Source(
        events=EventSource(
            table=get_staging_query_output_table_name(base_table),
            query=Query(
                selects=select(*columns),
                time_column="ts",
            ),
        )
    )
