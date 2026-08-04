import { useFieldRenderer } from '#core/components/form/hooks/use-field-renderer';

import type { FieldConfig } from '#core/components/form/types/config-types';

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
