import { StyledTab, StyledTabs } from './styled-components';

import type { DetailViewPagesProps } from '#core/components/views/detail-view/types/detail-view-component-types';

export function DetailViewPages({ tabs, activeTabId, onTabSelect }: DetailViewPagesProps) {
  const activeKey = activeTabId ?? tabs[0]?.id;

  if (!tabs || tabs.length === 0) {
    return <div>No tabs available</div>;
  }

  return (
    <StyledTabs
      activeKey={activeKey}
      onChange={({ activeKey }: { activeKey: string }) => onTabSelect?.(activeKey)}
    >
      {tabs.map((tab) => (
        <StyledTab key={tab.id} title={tab.label}>
          {tab.content}
        </StyledTab>
      ))}
    </StyledTabs>
  );
}
