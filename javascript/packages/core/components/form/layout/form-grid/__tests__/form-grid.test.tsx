import { render, screen } from '@testing-library/react';

import { FormGrid } from '#core/components/form/layout/form-grid/form-grid';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';

describe('FormGrid', () => {
  it('renders children', () => {
    render(
      <FormGrid>
        <span>Cell 1</span>
        <span>Cell 2</span>
      </FormGrid>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Cell 1')).toBeInTheDocument();
    expect(screen.getByText('Cell 2')).toBeInTheDocument();
  });
});
