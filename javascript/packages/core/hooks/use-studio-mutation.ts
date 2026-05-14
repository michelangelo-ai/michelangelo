import { useMutation } from '@tanstack/react-query';

import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { UseMutationResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';
import type { MutationConfig } from '#core/types/query-types';

export const useStudioMutation = <TData, TVariables extends Record<string, unknown>>(
  config: MutationConfig
): UseMutationResult<TData, ApplicationError, TVariables> => {
  const { mutationName, clientOptions } = config;
  const { request } = useServiceProvider();
  const normalizeError = useErrorNormalizer();

  return useMutation<TData, ApplicationError, TVariables>({
    mutationFn: async (variables: TVariables) => {
      try {
        return (await request(mutationName, variables)) as Promise<TData>;
      } catch (error) {
        console.error('mutation error', error);
        throw normalizeError(error)!;
      }
    },
    onSuccess: clientOptions?.onSuccess,
    onError: clientOptions?.onError,
  });
};
