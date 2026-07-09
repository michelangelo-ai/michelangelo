import { useMutation } from '@tanstack/react-query';

import { useSuccessOperations } from '#core/components/actions/use-success-operations';
import { useSchemaMiddleware } from '#core/hooks/use-schema-middleware/use-schema-middleware';
import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { ApplicationError } from '#core/types/error-types';
import type { MutationConfig } from '#core/types/query-types';
import type { UseStudioMutationResult } from './types';

export const useStudioMutation = <TResponse, TPayload extends Record<string, unknown>>(
  config: MutationConfig | null
): UseStudioMutationResult<TResponse, TPayload> => {
  const { request } = useServiceProvider();
  const normalizeError = useErrorNormalizer();
  const runSuccessOperations = useSuccessOperations(config?.successOperations);
  const { applyMiddleware } = useSchemaMiddleware(config?.middleware);

  const mutation = useMutation<TResponse, ApplicationError, TPayload>({
    mutationFn: async (payload: TPayload) => {
      if (!config) throw new Error('useStudioMutation called without config');
      try {
        // cast: service request returns unknown; TResponse is the caller-declared response type
        return (await request(config.mutationName, payload)) as TResponse;
      } catch (error) {
        console.error('mutation error', error);
        throw normalizeError(error)!;
      }
    },
    onSuccess: (data) => {
      runSuccessOperations(data);
      config?.clientOptions?.onSuccess?.(data);
    },
    onError: config?.clientOptions?.onError
      ? (error) => config.clientOptions!.onError!(error)
      : undefined,
  });

  return {
    isPending: mutation.isPending,
    error: mutation.error,
    data: mutation.data,
    mutate: (payload, options) =>
      mutation.mutate(applyMiddleware(payload, { sourceFromObject: options?.sourceFromObject })),
    mutateAsync: (payload, options) =>
      mutation.mutateAsync(
        applyMiddleware(payload, { sourceFromObject: options?.sourceFromObject })
      ),
  };
};
