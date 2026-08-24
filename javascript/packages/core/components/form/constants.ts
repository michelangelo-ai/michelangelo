import { SchemaBooleanField } from './fields/boolean/schema-boolean-field';
import { SchemaCheckboxField } from './fields/checkbox/schema-checkbox-field';
import { SchemaDateField } from './fields/date/schema-date-field';
import { SchemaMapField } from './fields/map/schema-map-field';
import { SchemaMarkdownField } from './fields/markdown/schema-markdown-field';
import { SchemaNumberField } from './fields/number/schema-number-field';
import { SchemaRadioField } from './fields/radio/schema-radio-field';
import { SchemaSelectField } from './fields/select/schema-select-field';
import { SchemaStringField } from './fields/string/schema-string-field';
import { SchemaTextareaField } from './fields/textarea/schema-textarea-field';
import { SchemaUrlField } from './fields/url/schema-url-field';

import type { FieldRenderer, FieldType } from './types/config-types';

/** Built-in field renderer registry. FormProvider renderers take priority over these. */
export const FIELD_RENDERERS: Record<FieldType, FieldRenderer> = {
  string: SchemaStringField,
  number: SchemaNumberField,
  boolean: SchemaBooleanField,
  select: SchemaSelectField,
  checkbox: SchemaCheckboxField,
  radio: SchemaRadioField,
  date: SchemaDateField,
  textarea: SchemaTextareaField,
  url: SchemaUrlField,
  map: SchemaMapField,
  markdown: SchemaMarkdownField,
};
