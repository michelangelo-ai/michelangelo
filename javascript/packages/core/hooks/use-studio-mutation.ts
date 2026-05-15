import { useMutation, useQueryClient } from '@tanstack/react-query';

import { useErrorNormalizer } from '#core/providers/error-provider/use-error-normalizer';
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

import type { UseMutationResult } from '@tanstack/react-query';
import type { ApplicationError } from '#core/types/error-types';
import type { MutationConfig } from '#core/types/query-types';

// Standard Kubernetes CRUD verbs. DeleteCollection must precede Delete so the
// regex matches it first (alternation is left-to-right).
const VERB_PATTERN = /^(DeleteCollection|Create|Update|Delete)(.+)$/;

export const useStudioMutation = <TData, TVariables extends Record<string, unknown>>(
  config: MutationConfig | null
): UseMutationResult<TData, ApplicationError, TVariables> => {
  const { request } = useServiceProvider();
  const normalizeError = useErrorNormalizer();
  const queryClient = useQueryClient();

  return useMutation<TData, ApplicationError, TVariables>({
    mutationFn: async (variables: TVariables) => {
      if (!config) throw new Error('useStudioMutation called without config');
      try {
        return (await request(config.mutationName, variables)) as Promise<TData>;
      } catch (error) {
        console.error('mutation error', error);
        throw normalizeError(error)!;
      }
    },
    onSuccess: config?.clientOptions?.onSuccess,
    onError: config?.clientOptions?.onError,
    // Auto-invalidation per ARCHITECTURE.md § 3 "Studio conventions encoded
    // in the runtime": derive the entity from `mutationName` and invalidate
    // its Get/List query keys on settle (success or failure).
    onSettled: () => {
      const entity = VERB_PATTERN.exec(config?.mutationName ?? '')?.[2];
      if (!entity) return;
      void queryClient.invalidateQueries({ queryKey: [`Get${entity}`] });
      void queryClient.invalidateQueries({ queryKey: [`List${entity}`] });
    },
  });
};
