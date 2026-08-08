import { BooleanField } from './boolean-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { BooleanFieldConfig } from './types';

export function SchemaBooleanField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing BooleanFieldConfig
  const c = config as BooleanFieldConfig;
  return (
    <BooleanField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      description={c.description}
      caption={c.caption}
      checkboxLabel={c.checkboxLabel}
      toggle={c.toggle}
    />
  );
}
