"""Book-level features for Amazon Books recommendation system.

Computes temporal features that enhance the dual-encoder model training
"""

from ai.chronon.api.ttypes import EventSource, Source
from ai.chronon.group_by import (
    Accuracy,
    Aggregation,
    GroupBy,
    Operation,
    TimeUnit,
    Window,
)
from ai.chronon.query import Query, select

# Removed imports that cause introspection issues:
# from ai.chronon.utils import get_staging_query_output_table_name
# from examples.amazon_books_qwen.data.staging_queries.amazon_books.books_reviews
#   import base_table

# Source for book popularity features
book_popularity_source = Source(
    events=EventSource(
        # Direct table name to avoid introspection issues
        table="amazon_books_books_reviews",
        query=Query(selects=select("book_id", "review_score"), time_column="ts"),
    )
)

# Book popularity and engagement features
book_popularity = GroupBy(
    sources=[book_popularity_source],
    keys=["book_id"],
    aggregations=[
        # Review count over different time windows
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating over time windows
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Rating variance (engagement quality indicator)
        Aggregation(
            input_column="review_score",
            operation=Operation.VARIANCE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Maximum and minimum ratings (range)
        Aggregation(
            input_column="review_score",
            operation=Operation.MAX,
            windows=[Window(length=30, timeUnit=TimeUnit.DAYS)],
        ),
        Aggregation(
            input_column="review_score",
            operation=Operation.MIN,
            windows=[Window(length=30, timeUnit=TimeUnit.DAYS)],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,  # Ensure training data reflects real-time behavior
)

# Source for review velocity features
review_velocity_source = Source(
    events=EventSource(
        # Direct table name to avoid introspection issues
        table="amazon_books_books_reviews",
        query=Query(selects=select("book_id", "1 AS review_event"), time_column="ts"),
    )
)

# Book review velocity (trending indicator)
book_velocity = GroupBy(
    sources=[review_velocity_source],
    keys=["book_id"],
    aggregations=[
        # Review velocity: count over shorter windows for trending
        Aggregation(
            input_column="review_event",
            operation=Operation.COUNT,
            windows=[
                Window(length=1, timeUnit=TimeUnit.DAYS),
                Window(length=3, timeUnit=TimeUnit.DAYS),
                Window(length=7, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Review acceleration: compare short vs medium term
        Aggregation(
            input_column="review_event",
            operation=Operation.SUM,
            windows=[
                Window(length=1, timeUnit=TimeUnit.DAYS),
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=14, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)
