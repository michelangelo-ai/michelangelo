import { render, screen } from '@testing-library/react';

import { GrpcStatusCode } from '#core/constants/grpc-status-codes';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getErrorProviderWrapper } from '#core/test/wrappers/get-error-provider-wrapper';
import { ApplicationError } from '#core/types/error-types';
import { useErrorNormalizer } from '../use-error-normalizer';

import type { ErrorNormalizer } from '#core/types/error-types';

// Test component that uses the error context
function TestComponent({ error }: { error: unknown }) {
  const normalizeError = useErrorNormalizer();
  const normalizedError = normalizeError(error);

  if (!normalizedError) {
    return <div>No error</div>;
  }

  return <div>{normalizedError.message}</div>;
}

describe('ErrorProvider', () => {
  it('should return No error when no error is falsy', () => {
    render(<TestComponent error={null} />, buildWrapper([getErrorProviderWrapper()]));

    expect(screen.getByText('No error')).toBeInTheDocument();
  });

  it('should use custom normalizer for application-specific errors', () => {
    const customNormalizer: ErrorNormalizer = (error: unknown) => {
      if (typeof error === 'object' && error !== null && 'customType' in error) {
        return new ApplicationError('Custom handled error', GrpcStatusCode.PERMISSION_DENIED, {
          source: 'custom-api',
        });
      }

      return null;
    };

    render(
      <TestComponent error={{ customType: 'MY_CUSTOM_ERROR' }} />,
      buildWrapper([getErrorProviderWrapper({ normalizeError: customNormalizer })])
    );

    expect(screen.getByText('Custom handled error')).toBeInTheDocument();
  });

  it('should fall back to default when custom normalizer returns null', () => {
    const customNormalizer: ErrorNormalizer = (error: unknown) => {
      if (typeof error === 'object' && error !== null && 'customType' in error) {
        return new ApplicationError('Custom handled error', GrpcStatusCode.INVALID_ARGUMENT, {
          source: 'custom-handler',
        });
      }

      return null;
    };

    const regularError = new Error('Regular error');

    render(
      <TestComponent error={regularError} />,
      buildWrapper([getErrorProviderWrapper({ normalizeError: customNormalizer })])
    );

    expect(screen.getByText('Regular error')).toBeInTheDocument();
    expect(screen.queryByText('Custom handled error')).not.toBeInTheDocument();
  });

  it('should throw error when used outside provider', () => {
    expect(() => {
      render(<TestComponent error={new Error('test')} />);
    }).toThrow('useErrorNormalizer must be used within an ErrorProvider');
  });
});
