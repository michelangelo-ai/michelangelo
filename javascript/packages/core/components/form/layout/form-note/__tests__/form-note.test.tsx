import { render, screen } from '@testing-library/react';

import { FormNote } from '#core/components/form/layout/form-note/form-note';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';

describe('FormNote', () => {
  it('renders markdown content as HTML', () => {
    render(<FormNote content="This is a **note**" />, buildWrapper([getBaseProviderWrapper()]));

    const bold = screen.getByText('note');
    expect(bold.tagName).toBe('STRONG');
  });

  it('renders plain text content', () => {
    render(<FormNote content="Just plain text" />, buildWrapper([getBaseProviderWrapper()]));

    expect(screen.getByText('Just plain text')).toBeInTheDocument();
  });
});
