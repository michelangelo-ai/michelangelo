import { render, screen } from '@testing-library/react';

import { FormStep } from '#core/components/form/layout/form-step/form-step';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';

describe('FormStep', () => {
  it('renders name heading and children', () => {
    render(
      <FormStep name="Model Configuration">
        <div>Field 1</div>
        <div>Field 2</div>
      </FormStep>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('Model Configuration')).toBeInTheDocument();
    expect(screen.getByText('Field 1')).toBeInTheDocument();
    expect(screen.getByText('Field 2')).toBeInTheDocument();
  });

  it('does not render description when not provided', () => {
    const { container } = render(
      <FormStep name="Step Title">
        <div>Content</div>
      </FormStep>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(container.querySelectorAll('p')).toHaveLength(0);
  });

  it('renders description as Markdown', () => {
    render(
      <FormStep name="Step Title" description="This is **important** context">
        <div>Content</div>
      </FormStep>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByText('important').tagName).toBe('STRONG');
  });
});
