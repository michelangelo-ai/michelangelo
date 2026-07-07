import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { TopBarNavButton } from '../top-bar-nav-button';

describe('TopBarNavButton', () => {
  it('renders the item label as button text', () => {
    render(<TopBarNavButton label="Docs" />, buildWrapper([getBaseProviderWrapper()]));

    expect(screen.getByRole('button', { name: 'Docs' })).toBeInTheDocument();
  });
});
