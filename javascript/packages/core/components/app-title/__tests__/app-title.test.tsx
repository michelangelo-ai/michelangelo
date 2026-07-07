import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { AppTitle } from '../app-title';

describe('AppTitle', () => {
  it('renders the given text as a link to /', async () => {
    render(<AppTitle>Michelangelo Studio</AppTitle>, buildWrapper([getRouterWrapper()]));

    const link = await screen.findByRole('link', { name: 'Michelangelo Studio' });
    expect(link).toHaveAttribute('href', '/');
  });
});
