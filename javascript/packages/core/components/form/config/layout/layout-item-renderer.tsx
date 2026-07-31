import { useLayoutRenderer } from '../hooks/use-layout-renderer';
import { BuiltinLayoutRenderer } from './builtin-layout-renderer';
import { LayoutItemList } from './layout-item-list';

import type { BuiltinLayoutConfig, FieldConfig, LayoutConfig } from '../types';

export function LayoutItemRenderer({
  config,
  entities,
}: {
  config: LayoutConfig;
  entities: Record<string, FieldConfig>;
}) {
  const CustomRenderer = useLayoutRenderer(config.type);

  if (CustomRenderer) {
    return (
      <CustomRenderer
        config={config}
        renderItems={(items) => <LayoutItemList items={items} entities={entities} />}
      />
    );
  }

  // cast: if no custom renderer matched, config is a built-in layout type
  return <BuiltinLayoutRenderer config={config as BuiltinLayoutConfig} entities={entities} />;
}
