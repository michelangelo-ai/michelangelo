import { SchemaField } from '#core/components/form/fields/schema-field';
import { BuiltinLayoutRenderer } from './builtin-layout-renderer';

import type {
  BuiltinLayoutConfig,
  FieldConfig,
  LayoutItem,
} from '#core/components/form/types/config-types';

/** Walks a layout tree, rendering bare strings as fields and objects as layout nodes. */
export function LayoutItemList({
  items,
  fields,
}: {
  items: LayoutItem[];
  fields: Record<string, FieldConfig | undefined>;
}) {
  return (
    <>
      {items.map((item, index) => {
        if (typeof item === 'string') {
          return <SchemaField key={item} fieldPath={item} config={fields[item]} />;
        }
        // cast: all layout types are built-in; consumer layout extensions are not supported
        const layoutConfig = item as BuiltinLayoutConfig;
        return <BuiltinLayoutRenderer key={index} config={layoutConfig} fields={fields} />;
      })}
    </>
  );
}
