/**
 * Core error interface that all error types should conform to
 * Uses gRPC status codes as the universal standard
 */
export interface ApplicationError {
  /** Human-readable error message */
  message: string;
  /** Error code - using gRPC codes as the standard */
  code: number;
  /** Optional additional context/metadata */
  meta?: Record<string, unknown>;
  /** Original error that caused this error */
  cause?: unknown;
  /** Error source/framework identifier */
  source?: string;
}

/**
 * Custom error normalizer function type
 * Applications provide this to handle their specific error types
 */
export type ErrorNormalizer = (error: unknown) => ApplicationError | null;
