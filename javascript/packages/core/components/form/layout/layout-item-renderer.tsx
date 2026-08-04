import { useLayoutRenderer } from '#core/components/form/hooks/use-layout-renderer';
import { BuiltinLayoutRenderer } from './builtin-layout-renderer';
import { LayoutItemList } from './layout-item-list';

import type {
  BuiltinLayoutConfig,
  FieldConfig,
  LayoutConfig,
} from '#core/components/form/types/config-types';

export function LayoutItemRenderer({
  config,
  fields,
}: {
  config: LayoutConfig;
  fields: Record<string, FieldConfig>;
}) {
  const CustomRenderer = useLayoutRenderer(config.type);

  if (CustomRenderer) {
    return (
      <CustomRenderer
        config={config}
        renderItems={(items) => <LayoutItemList items={items} fields={fields} />}
      />
    );
  }

  // cast: if no custom renderer matched, config is a built-in layout type
  return <BuiltinLayoutRenderer config={config as BuiltinLayoutConfig} fields={fields} />;
}
