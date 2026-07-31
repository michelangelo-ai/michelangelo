import { StringField } from '#core/components/form/fields/string/string-field';

import type { FieldRendererProps } from '#core/components/form/config/types';
import type { BaseFieldProps } from '#core/components/form/fields/types';

export function SchemaStringField({ config, ...baseProps }: FieldRendererProps) {
  if (config.multi) {
    // cast: multi=true narrows StringField to the string[] variant
    return <StringField {...(baseProps as BaseFieldProps<string[]>)} multi />;
  }
  // cast: multi=false/absent narrows StringField to the string variant
  return <StringField {...(baseProps as BaseFieldProps<string>)} />;
}
