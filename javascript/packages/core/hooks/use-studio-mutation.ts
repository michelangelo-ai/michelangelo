import { useMutation } from '@tanstack/react-query';

import { useSuccessOperations } from '#core/components/actions/use-success-operations';
import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { UseMutationResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';
import type { MutationConfig } from '#core/types/query-types';

export const useStudioMutation = <TData, TVariables extends Record<string, unknown>>(
  config: MutationConfig | null
): UseMutationResult<TData, ApplicationError, TVariables> => {
  const { request } = useServiceProvider();
  const normalizeError = useErrorNormalizer();
  const runSuccessOperations = useSuccessOperations(config?.successOperations);

  return useMutation<TData, ApplicationError, TVariables>({
    mutationFn: async (variables: TVariables) => {
      if (!config) throw new Error('useStudioMutation called without config');
      try {
        // cast: service request returns unknown; TData is the caller-declared response type
        return (await request(config.mutationName, variables)) as TData;
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
};
