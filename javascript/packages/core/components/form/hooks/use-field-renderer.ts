import { FIELD_RENDERERS } from '#core/components/form/constants';
import { useFormProvider } from '#core/providers/form-provider/use-form-provider';

import type { FieldRenderer } from '#core/components/form/types/config-types';

export function useFieldRenderer(type: string): FieldRenderer | undefined {
  const formContext = useFormProvider();
  // cast: FIELD_RENDERERS is keyed by FieldType enum but we look up by arbitrary string
  return formContext?.renderers[type] ?? (FIELD_RENDERERS as Record<string, FieldRenderer>)[type];
}
