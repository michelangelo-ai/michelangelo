import { RadioField } from './radio-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { RadioFieldConfig } from './types';

export function SchemaRadioField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing RadioFieldConfig
  const c = config as RadioFieldConfig;
  return (
    <RadioField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      description={c.description}
      caption={c.caption}
      options={c.options ?? []}
      align={c.align}
    />
  );
}
