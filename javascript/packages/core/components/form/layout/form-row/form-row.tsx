import React from 'react';
import { useStyletron } from 'baseui';
import { Cell, Grid } from 'baseui/layout-grid';

import type { FormRowProps } from './types';

export const FormRow: React.FC<FormRowProps> = ({ name, span, children }) => {
  const [css, theme] = useStyletron();
  const childrenArray = React.Children.toArray(children);
  const columns = span?.reduce((a, b) => a + b, 0) ?? childrenArray.length;

  return (
    <div
      className={css({
        flexGrow: 1,
        boxSizing: 'border-box',
      })}
    >
      {name && <div className={css(theme.typography.LabelMedium)}>{name}</div>}

      <Grid gridColumns={columns} gridMargins={[0]} gridMaxWidth={0}>
        {childrenArray.map((child, i) => (
          <Cell key={i} span={span?.[i]}>
            {child}
          </Cell>
        ))}
      </Grid>
    </div>
  );
};
