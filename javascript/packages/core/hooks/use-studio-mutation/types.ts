import type { ApplicationError } from '#core/types/error-types';

export type StudioMutateOptions<TPayload> = {
  /** Read middleware `source` paths from this object instead of the submitted payload. */
  sourceFromObject?: TPayload;
};

export type UseStudioMutationResult<TResponse, TPayload extends Record<string, unknown>> = {
  isPending: boolean;
  error: ApplicationError | null;
  data: TResponse | undefined;
  mutate: (payload: TPayload, options?: StudioMutateOptions<TPayload>) => void;
  mutateAsync: (payload: TPayload, options?: StudioMutateOptions<TPayload>) => Promise<TResponse>;
};
