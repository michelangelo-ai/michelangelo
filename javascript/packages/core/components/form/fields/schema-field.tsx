import { memo } from 'react';

import { useFieldRenderer } from '#core/components/form/hooks/use-field-renderer';

import type { FieldConfig } from '#core/components/form/types/config-types';

/**
 * Resolves and renders the field renderer for a given field path.
 *
 * Renders nothing if specified field configuration's type is not
 * registered to FIELD_RENDERERS or FormContext.renderers.
 *
 * Wrapped in `memo` so that a stable `config` reference (see `ResolvedFormContent`)
 * skips re-rendering fields whose resolved interpolations haven't changed.
 */
function SchemaFieldInner({
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

export const SchemaField = memo(SchemaFieldInner);
