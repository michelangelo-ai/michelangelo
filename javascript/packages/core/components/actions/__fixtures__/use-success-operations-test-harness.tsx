import { useSuccessOperations } from '#core/components/actions/use-success-operations';

import type { SuccessOperation } from '#core/components/actions/types';

type HarnessProps = {
  operations?: SuccessOperation[];
  response?: unknown;
  mutationName?: string;
};

export function UseSuccessOperationsTestHarness({
  operations,
  response,
  mutationName,
}: HarnessProps) {
  const runSuccessOperations = useSuccessOperations(operations, mutationName);

  return <button onClick={() => runSuccessOperations(response)}>Run success operations</button>;
}
