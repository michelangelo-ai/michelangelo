import { DateField } from './date-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { DateFieldConfig } from './types';

export function SchemaDateField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing DateFieldConfig
  const c = config as DateFieldConfig;
  return (
    <DateField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      placeholder={c.placeholder}
      description={c.description}
      caption={c.caption}
      dateFormat={c.dateFormat}
      noFutureDate={c.noFutureDate}
    />
  );
}
