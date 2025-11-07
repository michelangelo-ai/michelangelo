"""
Join definition for Amazon Books recommendation features
Combines all feature GroupBys into a comprehensive feature set for ML training
"""

from ai.chronon.join import Join, JoinPart
from examples.amazon_books_qwen.data.group_bys.amazon_books.book_features import (
    book_popularity,
    book_velocity,
)
from examples.amazon_books_qwen.data.group_bys.amazon_books.user_features import (
    user_reading_patterns,
    user_genre_preferences,
)
from examples.amazon_books_qwen.data.group_bys.amazon_books.content_features import (
    author_features,
    category_features,
    publisher_features,
)
from examples.amazon_books_qwen.data.sources.amazon_books.books import books_source

# Main training set join for recommendation features
recommendation_features = Join(
    left=books_source(
        "book_id",
        "book_title",
        "book_authors",
        "book_categories",
        "book_publisher",
        "book_published_date",
        "book_description",
        "review_id",
        "review_score",
        "review_summary",
        "review_text",
    ),
    right_parts=[
        # Book-level features
        JoinPart(group_by=book_popularity),
        JoinPart(group_by=book_velocity),
        # Content-based features
        JoinPart(group_by=author_features),
        JoinPart(group_by=category_features),
        JoinPart(group_by=publisher_features),
        # User behavior features (conceptual - requires user tracking)
        JoinPart(group_by=user_reading_patterns),
        JoinPart(group_by=user_genre_preferences),
    ],
)

# Specialized join for book-centric features only (for inference)
book_features_only = Join(
    left=books_source(
        "book_id",
        "book_title",
        "book_authors",
        "book_categories",
        "book_publisher",
        "book_description",
    ),
    right_parts=[
        JoinPart(group_by=book_popularity),
        JoinPart(group_by=book_velocity),
        JoinPart(group_by=author_features),
        JoinPart(group_by=category_features),
        JoinPart(group_by=publisher_features),
    ],
)

# Content similarity features for cold-start recommendations
content_similarity_features = Join(
    left=books_source(
        "book_id", "book_title", "book_authors", "book_categories", "book_description"
    ),
    right_parts=[
        JoinPart(group_by=author_features),
        JoinPart(group_by=category_features),
        JoinPart(group_by=publisher_features),
    ],
)
