"""User behavior features for Amazon Books recommendation system.

Captures user reading patterns and preferences for personalization
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
from ai.chronon.utils import get_staging_query_output_table_name

from examples.amazon_books_qwen.data.staging_queries.amazon_books.books_reviews import (
    base_table,
)

# Note: In the current Amazon Books dataset, we don't have explicit user IDs
# This is a conceptual implementation for when user data becomes available
# For now, we'll use review patterns as a proxy for user behavior

# Source for user rating patterns (using review patterns as proxy)
user_rating_source = Source(
    events=EventSource(
        table=get_staging_query_output_table_name(base_table),
        query=Query(
            # In real implementation, would use user_id; here using review patterns
            selects=select("review_id", "review_score", "book_categories"),
            time_column="ts",
        ),
    )
)

# User reading behavior patterns (conceptual - requires user tracking)
user_reading_patterns = GroupBy(
    sources=[user_rating_source],
    keys=["review_id"],  # Would be user_id in real implementation
    aggregations=[
        # Reading frequency
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating given by user (user preference indicator)
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Rating variance (how selective the user is)
        Aggregation(
            input_column="review_score",
            operation=Operation.VARIANCE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)

# Genre preference tracking (conceptual)
genre_preference_source = Source(
    events=EventSource(
        table=get_staging_query_output_table_name(base_table),
        query=Query(
            selects=select("review_id", "book_categories", "review_score"),
            time_column="ts",
        ),
    )
)

# User genre preferences (would require user segmentation)
user_genre_preferences = GroupBy(
    sources=[genre_preference_source],
    keys=[
        "review_id",
        "book_categories",
    ],  # Would be user_id, genre in real implementation
    aggregations=[
        # Books read per genre
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating per genre
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)
