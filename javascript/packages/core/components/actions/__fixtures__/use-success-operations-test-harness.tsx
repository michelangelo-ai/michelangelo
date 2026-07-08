import { useSuccessOperations } from '#core/components/actions/use-success-operations';

import type { SuccessOperation } from '#core/components/actions/types';

type HarnessProps = {
  operations?: SuccessOperation[];
  response?: unknown;
};

export function UseSuccessOperationsTestHarness({ operations, response }: HarnessProps) {
  const runSuccessOperations = useSuccessOperations(operations);

  return <button onClick={() => runSuccessOperations(response)}>Run success operations</button>;
}
