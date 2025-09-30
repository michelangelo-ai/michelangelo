import { render, screen } from '@testing-library/react';

import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';

describe('FormRow', () => {
  it('renders children in a grid layout with optional name label', () => {
    render(
      <FormRow name="Contact Information">
        <div>Field 1</div>
        <div>Field 2</div>
        <div>Field 3</div>
      </FormRow>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Contact Information')).toBeInTheDocument();
    expect(screen.getByText('Field 1')).toBeInTheDocument();
    expect(screen.getByText('Field 2')).toBeInTheDocument();
    expect(screen.getByText('Field 3')).toBeInTheDocument();
  });
});
