import { SchemaField } from '#core/components/form/fields/schema-field';
import { LayoutRenderer } from './layout-renderer';

import type { FieldConfig, LayoutItem } from '#core/components/form/types/config-types';

/** Renders a list of layout items — strings become fields, objects become layout components. */
export function LayoutItemList({
  items,
  fields,
}: {
  items: LayoutItem[];
  /** Field configs keyed by path — untyped since layouts are structural and don't depend on the data shape. */
  fields: Record<string, FieldConfig | undefined>;
}) {
  return (
    <>
      {items.map((item, index) => {
        if (typeof item === 'string') {
          return <SchemaField key={item} fieldPath={item} config={fields[item]} />;
        }
        return <LayoutRenderer key={index} config={item} fields={fields} />;
      })}
    </>
  );
}
