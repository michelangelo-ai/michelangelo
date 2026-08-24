import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { MarkdownField } from './markdown-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { MarkdownFieldConfig } from './types';

export function SchemaMarkdownField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing MarkdownFieldConfig
  const c = config as MarkdownFieldConfig;
  const validate = resolveValidation(c.validation);
  return (
    <MarkdownField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
      rows={c.rows}
      maxLength={c.maxLength}
      validate={validate}
    />
  );
}
