import { NumberField } from './number-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { NumberFieldConfig } from './types';

export function SchemaNumberField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing NumberFieldConfig
  const c = config as NumberFieldConfig;
  return (
    <NumberField
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
