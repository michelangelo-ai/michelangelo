import { HelpTooltip } from '#core/components/help-tooltip';

import type { SharedCell } from '#core/components/cell/types';

export const CellEnhancer = ({ endEnhancer }: { endEnhancer?: SharedCell['endEnhancer'] }) => {
  if (!endEnhancer) {
    return null;
  }

  switch (endEnhancer.type) {
    case 'tooltip':
      return <HelpTooltip text={endEnhancer.content} />;
    default:
      return null;
  }
};
