import { useStyletron } from 'baseui';

import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { Icon } from '#core/components/icon/icon';
import { Link } from '#core/components/link/link';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRendererProps } from '#core/components/cell/types';
import type { LinkCellConfig } from './types';

/**
 * Cell renderer for clickable links with optional icon.
 *
 * Renders text as a link when column.url is provided, otherwise displays plain text.
 * Supports optional leading icon and automatic text truncation.
 *
 * @param props.value - Text value to display
 * @param props.column - Column configuration with optional url and icon
 *   - column.url: Link destination
 *   - column.icon: Icon name to show before the text
 *
 * @example
 * ```tsx
 * // In table column definition
 * {
 *   id: 'name',
 *   label: 'Pipeline',
 *   type: CellType.LINK,
 *   url: '/pipelines/${row.id}',
 *   icon: 'pipeline'
 * }
 * // Renders: [icon] <a href="/pipelines/123">My Pipeline</a>
 * ```
 */
export function LinkCell(props: CellRendererProps<string, LinkCellConfig>) {
  const [css, theme] = useStyletron();
  const cellToString = useCellToString();
  const { column } = props;
  const { icon, url } = column;

  const content = <TruncatedText>{cellToString(props) ?? props.value}</TruncatedText>;

  return (
    <div className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale100 })}>
      {icon && <Icon name={icon} />}
      {url && typeof url === 'string' ? <Link href={url}>{content}</Link> : content}
    </div>
  );
}
