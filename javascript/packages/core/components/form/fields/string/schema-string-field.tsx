import { StringField } from './string-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { StringFieldConfig } from './types';

/** Schema-aware wrapper for StringField that maps config to component props. */
export function SchemaStringField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing StringFieldConfig
  const c = config as StringFieldConfig;

  if (c.multi) {
    return (
      <StringField
        name={name}
        label={c.label}
        required={c.required}
        disabled={c.disabled}
        readOnly={c.readOnly}
        placeholder={c.placeholder}
        description={c.description}
        caption={c.caption}
        multi
      />
    );
  }
  return (
    <StringField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
    />
  );
}
