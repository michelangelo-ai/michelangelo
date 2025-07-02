import { useStyletron } from 'baseui';

import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { Icon } from '#core/components/icon/icon';
import { Link } from '#core/components/link/link';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRendererProps } from '#core/components/cell/types';
import type { LinkCellConfig } from './types';

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
