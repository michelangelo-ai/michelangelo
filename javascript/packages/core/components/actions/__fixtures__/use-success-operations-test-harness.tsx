import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { useSuccessOperations } from '#core/components/actions/use-success-operations';

import type { QueryClient } from '@tanstack/react-query';
import type { SuccessOperation } from '#core/components/actions/types';

type HarnessProps = {
  operations?: SuccessOperation[];
  response?: unknown;
};

export function UseSuccessOperationsTestHarness({ operations, response }: HarnessProps) {
  const runSuccessOperations = useSuccessOperations(operations);

  return <button onClick={() => runSuccessOperations(response)}>Run success operations</button>;
}

export function QueryClientCapture({ onReady }: { onReady: (queryClient: QueryClient) => void }) {
  const queryClient = useQueryClient();

  useEffect(() => {
    onReady(queryClient);
  }, [onReady, queryClient]);

  return null;
}
