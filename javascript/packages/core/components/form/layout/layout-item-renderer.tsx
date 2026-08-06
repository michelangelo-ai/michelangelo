import { BuiltinLayoutRenderer } from './builtin-layout-renderer';

import type {
  BuiltinLayoutConfig,
  FieldConfig,
  LayoutConfig,
} from '#core/components/form/types/config-types';

/** Renders a layout config node using the built-in layout renderer. */
export function LayoutItemRenderer({
  config,
  fields,
}: {
  config: LayoutConfig;
  fields: Record<string, FieldConfig | undefined>;
}) {
  // cast: all layout types are built-in; consumer layout extensions are not supported
  return <BuiltinLayoutRenderer config={config as BuiltinLayoutConfig} fields={fields} />;
}
