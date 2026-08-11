import { FIELD_RENDERERS } from '#core/components/form/constants';
import { useFormProvider } from '#core/providers/form-provider/use-form-provider';

import type { FieldRenderer } from '#core/components/form/types/config-types';

/**
 * Resolves a field renderer by type. Checks context-registered renderers first
 * (via FormProvider), then falls back to built-in renderers.
 */
export function useFieldRenderer(type: string): FieldRenderer | undefined {
  const formContext = useFormProvider();
  // cast: FIELD_RENDERERS is keyed by FieldType enum but we look up by arbitrary string
  return formContext?.renderers[type] ?? (FIELD_RENDERERS as Record<string, FieldRenderer>)[type];
}
