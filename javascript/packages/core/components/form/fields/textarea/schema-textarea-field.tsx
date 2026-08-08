import { TextareaField } from './textarea-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { TextareaFieldConfig } from './types';

export function SchemaTextareaField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing TextareaFieldConfig
  const c = config as TextareaFieldConfig;
  return (
    <TextareaField
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
    />
  );
}
