import { useCallback } from 'react';
import { useNavigate } from 'react-router-dom-v5-compat';
import { useQueryClient } from '@tanstack/react-query';
import { useSnackbar } from 'baseui/snackbar';

import { Icon } from '#core/components/icon/icon';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';

import type { SuccessOperation, ToastOperation } from './types';

const DEFAULT_TOAST_ICON = 'checkCircle';

/**
 * Returns a runner function that processes a mutation's `successOperations`.
 *
 * The runner resolves response interpolation (e.g. `${response.metadata.name}`)
 * against the mutation result, then applies each operation in order.
 */
export function useSuccessOperations(operations?: SuccessOperation[]) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { enqueue, dequeue } = useSnackbar();
  const resolver = useInterpolationResolver();

  return useCallback(
    (response: unknown) => {
      if (!operations || operations.length === 0) return;
      const resolved = resolver(operations, { response });
      for (const op of resolved) {
        if (op.type === 'invalidate') {
          for (const target of op.targets) {
            const queryKey =
              typeof target === 'string' ? [target] : [target.name, target.serviceOptions];
            void queryClient.invalidateQueries({ queryKey });
          }
        } else if (op.type === 'toast') {
          enqueue(buildToastPayload(op, navigate, dequeue));
        }
      }
    },
    [operations, resolver, queryClient, navigate, enqueue, dequeue]
  );
}

function buildToastPayload(
  op: ToastOperation,
  navigate: ReturnType<typeof useNavigate>,
  dequeue: () => void
) {
  const StartEnhancer = ({ size }: { size: number }) => (
    <Icon name={op.icon ?? DEFAULT_TOAST_ICON} size={`${size}px`} />
  );
  if (op.action) {
    const { label, route } = op.action;
    return {
      message: op.message,
      startEnhancer: StartEnhancer,
      actionMessage: label,
      actionOnClick: () => (route ? navigate(route) : dequeue()),
    };
  }
  return { message: op.message, startEnhancer: StartEnhancer };
}
