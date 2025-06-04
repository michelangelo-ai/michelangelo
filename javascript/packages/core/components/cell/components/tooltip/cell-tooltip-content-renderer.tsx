import { Markdown } from '#core/components/markdown/markdown';

import type { TooltipHOCProps } from './types';

export function CellTooltipContentRenderer(props: TooltipHOCProps<unknown>) {
  if (typeof props.column.tooltip.content === 'function') {
    const Component = props.column.tooltip.content;
    return <Component {...props} />;
  }

  return <Markdown>{props.column.tooltip.content}</Markdown>;
}
