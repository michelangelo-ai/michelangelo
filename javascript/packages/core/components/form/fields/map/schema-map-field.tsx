import { resolveValidation } from '#core/components/form/validation/resolve-validation';
import { MapField } from './map-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';
import type { MapFieldConfig } from './types';

export function SchemaMapField({ name, config }: FieldRendererProps) {
  // cast: SchemaField routes by config.type, guaranteeing MapFieldConfig
  const c = config as MapFieldConfig;
  const validate = resolveValidation(c.validation);
  return (
    <MapField
      name={name}
      label={c.label}
      required={c.required}
      disabled={c.disabled}
      readOnly={c.readOnly}
      description={c.description}
      caption={c.caption}
      singleValue={c.singleValue}
      creatable={c.creatable}
      deletable={c.deletable}
      emptyMessage={c.emptyMessage}
      keyConfig={c.keyConfig}
      valueConfig={c.valueConfig}
      size={c.size}
      validate={validate}
    />
  );
}
