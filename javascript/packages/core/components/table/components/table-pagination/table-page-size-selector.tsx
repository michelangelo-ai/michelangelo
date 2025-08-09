import { useStyletron } from 'baseui';
import { Select } from 'baseui/select';

import type { Theme } from 'baseui/theme';
import type { TablePaginationProps } from './types';

export function TablePageSizeSelector(
  props: Pick<TablePaginationProps, 'pageSizes' | 'state' | 'setPageSize'>
) {
  const { pageSizes, state, setPageSize } = props;
  const { pageSize } = state;
  const [css, theme] = useStyletron();

  const selectedOption = pageSizes.find((option) => option.id === pageSize);

  return (
    <div
      className={css({
        ...theme.typography.LabelMedium,
        display: 'flex',
        gap: theme.sizing.scale600,
        alignItems: 'center',
      })}
    >
      <div className={css({ minWidth: 'fit-content', paddingLeft: theme.sizing.scale600 })}>
        Rows per page
      </div>
      <Select
        placeholder=""
        aria-label="Select rows per page"
        clearable={false}
        searchable={false}
        options={pageSizes}
        value={selectedOption ? [selectedOption] : []}
        onChange={({ option }) => setPageSize(Number(option?.id ?? pageSize))}
        overrides={{
          Root: {
            props: {
              'aria-label': 'Select rows per page',
            },
          },
          ControlContainer: {
            style: ({ $theme }: { $theme: Theme }) => ({
              borderColor: 'transparent',
              backgroundColor: $theme.colors.buttonTertiaryFill,
              ':hover': {
                backgroundColor: $theme.colors.buttonTertiaryHover,
              },
            }),
          },
          SingleValue: {
            style: ({ $theme }: { $theme: Theme }) => ({
              color: $theme.colors.buttonTertiaryText,
              paddingLeft: $theme.sizing.scale200,
              paddingRight: $theme.sizing.scale500,
            }),
          },
        }}
      />
    </div>
  );
}
