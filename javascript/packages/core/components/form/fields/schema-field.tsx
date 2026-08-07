import { useFieldRenderer } from '#core/components/form/hooks/use-field-renderer';

import type { FieldConfig } from '#core/components/form/types/config-types';

/**
 * Resolves and renders the field renderer for a given field path.
 *
 * Renders nothing if specified field configuration's type is not
 * registered to FIELD_RENDERERS or FormContext.renderers.
 */
export function SchemaField({
  fieldPath,
  config,
}: {
  fieldPath: string;
  config: FieldConfig | undefined;
}) {
  const Renderer = useFieldRenderer(config?.type ?? '');
  if (!Renderer || !config) return null;

  return <Renderer name={fieldPath} config={config} />;
}
