import { StringField } from './string-field';

import type { FieldRendererProps } from '#core/components/form/types/config-types';

export function SchemaStringField({ name, config }: FieldRendererProps) {
  if ('multi' in config && config.multi) {
    return (
      <StringField
        name={name}
        label={config.label}
        required={config.required}
        disabled={config.disabled}
        readOnly={config.readOnly}
        placeholder={config.placeholder}
        description={config.description}
        caption={config.caption}
        multi
      />
    );
  }
  return (
    <StringField
      name={name}
      label={config.label}
      required={config.required}
      disabled={config.disabled}
      readOnly={config.readOnly}
      placeholder={config.placeholder}
      description={config.description}
      caption={config.caption}
    />
  );
}
