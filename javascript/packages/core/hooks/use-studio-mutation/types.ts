import type { UseMutationResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';

export type StudioMutateOptions<TVariables> = {
  /** Read middleware `source` paths from this object instead of the submitted variables. */
  sourceFromObject?: TVariables;
};

export type UseStudioMutationResult<TData, TVariables extends Record<string, unknown>> = Omit<
  UseMutationResult<TData, ApplicationError, TVariables>,
  'mutate' | 'mutateAsync'
> & {
  mutate: (variables: TVariables, options?: StudioMutateOptions<TVariables>) => void;
  mutateAsync: (variables: TVariables, options?: StudioMutateOptions<TVariables>) => Promise<TData>;
};
