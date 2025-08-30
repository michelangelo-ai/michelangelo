import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';
import { Checkbox } from 'baseui/checkbox';
import { List, SharedStylePropsArg, StyledCloseHandle, StyledLabel } from 'baseui/dnd-list';
import { PLACEMENT, StatefulPopover } from 'baseui/popover';

import { Icon } from '#core/components/icon/icon';
import { TableData } from '#core/components/table/types/data-types';
import { createColumnListChangeHandler } from './utils';

import type { ConfigurableColumn, TableColumnConfigurationButtonProps } from './types';

export function TableColumnConfigurationButton<T extends TableData = TableData>({
  columns,
  setColumnOrder,
  setColumnVisibility,
}: TableColumnConfigurationButtonProps<T>) {
  const [css, theme] = useStyletron();

  return (
    <StatefulPopover
      placement={PLACEMENT.bottomRight}
      content={() => (
        <div
          className={css({
            backgroundColor: theme.colors.backgroundPrimary,
            borderRadius: theme.borders.radius300,
            width: '214px',
            display: 'flex',
            flexDirection: 'column',
          })}
        >
          <List
            removable
            overrides={{
              Item: { style: { height: theme.sizing.scale800 } },
              Label: {
                component: (props: { $value: ConfigurableColumn<T> } & SharedStylePropsArg) => (
                  <StyledLabel {...props}>{props.$value.label}</StyledLabel>
                ),
              },
              CloseHandle: {
                component: (props: { $value: ConfigurableColumn<T> } & SharedStylePropsArg) => (
                  <StyledCloseHandle {...props}>
                    <Checkbox
                      checked={props.$value.isVisible}
                      disabled={!props.$value.canHide}
                      title={`Toggle ${props.$value.label}`}
                    />
                  </StyledCloseHandle>
                ),
              },
            }}
            onChange={createColumnListChangeHandler(columns, setColumnOrder, setColumnVisibility)}
            // @ts-expect-error Items are expected to be React Nodes, but we set them as objects
            // and with the help of overrides, we render them as React Nodes

            // MA Studio does not support reordering or hiding the first column, since it is usually
            // the unique identifier column.
            items={columns.slice(1)}
          />
        </div>
      )}
    >
      <Button title="Configure columns" size={SIZE.mini} kind={KIND.tertiary}>
        <Icon name="settings" />
      </Button>
    </StatefulPopover>
  );
}
