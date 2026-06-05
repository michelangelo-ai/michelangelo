import { useMutation, useQueryClient } from '@tanstack/react-query';

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
    onSuccess: config?.clientOptions?.onSuccess
      ? (data) => config.clientOptions!.onSuccess!(data)
      : undefined,
    onError: config?.clientOptions?.onError
      ? (error) => config.clientOptions!.onError!(error)
      : undefined,
    // The API server names handlers as {Verb}{Kind} (Kubernetes convention);
    // strip the verb to derive the kind and invalidate Get{Kind}+List{Kind}
    // on every settle so reads stay consistent without per-call declarations.
    onSettled: () => {
      const entity = k8sEntityFromMutationName(config?.mutationName ?? '');
      if (!entity) return;
      void queryClient.invalidateQueries({ queryKey: [`Get${entity}`] });
      void queryClient.invalidateQueries({ queryKey: [`List${entity}`] });
    },
  });
};

// Kubernetes CRUD verbs — DeleteCollection precedes Delete to avoid prefix collision.
const K8S_CRUD_VERB = /^(?<verb>DeleteCollection|Create|Update|Delete)(?<entity>.+)$/;

function k8sEntityFromMutationName(mutationName: string): string | undefined {
  return K8S_CRUD_VERB.exec(mutationName)?.groups?.entity;
}
