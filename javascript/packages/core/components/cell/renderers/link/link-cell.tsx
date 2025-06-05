import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';
import { Link } from '#core/components/link/link';
import { linkCellToString } from './link-cell-to-string';

import type { CellRendererProps } from '#core/components/cell/types';
import type { LinkCellConfig } from './types';

export function LinkCell(props: CellRendererProps<string, LinkCellConfig>) {
  const [css, theme] = useStyletron();
  const { column } = props;
  const { icon, url } = column;

  return (
    <div className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale100 })}>
      {icon && <Icon name={icon} />}
      {url && typeof url === 'string' ? (
        <Link href={url}>{linkCellToString(props)}</Link>
      ) : (
        linkCellToString(props)
      )}
    </div>
  );
}
