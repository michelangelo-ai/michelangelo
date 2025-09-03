import { mergeOverrides, type Theme } from 'baseui';
import { Tab, Tabs } from 'baseui/tabs-motion';

import type { TabProps, TabsProps } from 'baseui/tabs-motion';

export function StyledTabs(props: TabsProps) {
  const overrides = mergeOverrides(props.overrides, {
    Root: { style: { transform: 'unset' } },
    TabBorder: { style: { zIndex: -1 } },
    TabHighlight: { style: { zIndex: 0 } },
  });

  return <Tabs {...props} overrides={overrides} />;
}

export function StyledTab(props: TabProps) {
  const overrides = mergeOverrides(props.overrides, {
    TabPanel: {
      style: ({ $theme }: { $theme: Theme }) => ({
        paddingTop: $theme.sizing.scale950,
        paddingLeft: 0,
        paddingRight: 0,
      }),
    },
  });
  return <Tab {...props} overrides={overrides} />;
}
