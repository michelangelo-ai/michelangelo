import type { SuccessOperation } from '#core/components/actions/types';
import type { FormOperation } from '#core/components/views/types';
import type { MiddlewareSchema } from '#core/hooks/use-schema-middleware/types';
import type { MutationConfig } from '#core/types/query-types';

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function buildFormMutationConfig({
  service,
  formOperation,
  successOperations,
  middleware,
}: {
  service: string;
  formOperation: FormOperation;
  successOperations?: SuccessOperation[];
  middleware?: MiddlewareSchema;
}): MutationConfig {
  return {
    mutationName: `${capitalize(formOperation)}${capitalize(service)}`,
    successOperations,
    middleware,
  };
}
