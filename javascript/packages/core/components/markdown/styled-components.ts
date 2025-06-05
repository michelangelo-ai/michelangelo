import { styled } from 'baseui';
import {
  StyledTable,
  StyledTableBody,
  StyledTableBodyCell,
  StyledTableBodyRow,
  StyledTableHead,
  StyledTableHeadCell,
} from 'baseui/table-semantic';

import { Link } from '#core/components/link/link';

export const MarkdownParagraph = styled('p', ({ $theme }) => ({
  fontSize: 'inherit',
  lineHeight: 'inherit',
  whiteSpace: 'pre-wrap',
  marginBlockStart: $theme.sizing.scale200,
  marginBlockEnd: $theme.sizing.scale200,
}));

export const MarkdownListItem = styled('li', {
  fontSize: 'inherit',
  lineHeight: 'inherit',
});

export const MarkdownUnorderedList = styled('ul', ({ $theme }) => ({
  paddingInlineStart: $theme.sizing.scale600,
}));

export const MarkdownStrong = styled('strong', ({ $theme }) => ({
  ...$theme.typography.LabelXSmall,
  fontSize: 'inherit',
}));

/**
 * @important Component overrides must inherit `color`-like styles from parent.
 * @example when used within a `<Tooltip />` component, they can display `color: #fff`
 */
export const MARKDOWN_OVERRIDES = {
  a: { component: Link },
  p: { component: MarkdownParagraph },
  strong: { component: MarkdownStrong },
  ul: { component: MarkdownUnorderedList },
  li: { component: MarkdownListItem },
  table: { component: StyledTable },
  td: { component: StyledTableBodyCell },
  tr: { component: StyledTableBodyRow },
  th: { component: StyledTableHeadCell },
  tbody: { component: StyledTableBody },
  thead: { component: StyledTableHead },
};
