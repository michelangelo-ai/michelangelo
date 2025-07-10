/**
 * Core error class that all error types should extend
 * Uses gRPC status codes as the universal standard
 */
export class ApplicationError extends Error {
  name = 'ApplicationError' as const;
  /** Error code - using gRPC codes as the standard */
  code: number;
  /** Optional additional context/metadata */
  meta?: Record<string, unknown>;
  /** Original error that caused this error */
  cause?: unknown;
  /** Error source/framework identifier */
  source?: string;

  constructor(
    message: string,
    code: number,
    options?: {
      meta?: Record<string, unknown>;
      cause?: unknown;
      source?: string;
    }
  ) {
    super(message);
    this.code = code;
    this.meta = options?.meta;
    this.cause = options?.cause;
    this.source = options?.source;

    // Ensure prototype chain is correct for instanceof
    Object.setPrototypeOf(this, ApplicationError.prototype);

    // Improve stack traces (V8 only)
    Error.captureStackTrace?.(this, this.constructor);
  }
}

/**
 * Custom error normalizer function type
 * Applications provide this to handle their specific error types
 */
export type ErrorNormalizer = (error: unknown) => ApplicationError | null;
