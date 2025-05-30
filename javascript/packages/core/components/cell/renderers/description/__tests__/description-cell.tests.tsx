import { render, screen } from '@testing-library/react';

import { CellType } from '#core/components/cell/constants';
import { DescriptionCell } from '../description-cell';

describe('DescriptionColumn', () => {
  test('Renders provided text', async () => {
    render(
      <DescriptionCell
        column={{ id: 'spec.description', type: CellType.DESCRIPTION }}
        record={{ spec: { description: 'Descriptive text in the column' } }}
        value={'Descriptive text in the column'}
      />
    );

    await screen.findByText('Descriptive text in the column');
  });
});
