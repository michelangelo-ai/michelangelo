import { render, screen } from '@testing-library/react';

import { FormColumn } from '#core/components/form/layout/form-column/form-column';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';

describe('FormColumn', () => {
  it('renders children', () => {
    render(
      <FormColumn>
        <span>Column content</span>
      </FormColumn>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Column content')).toBeInTheDocument();
  });
});
