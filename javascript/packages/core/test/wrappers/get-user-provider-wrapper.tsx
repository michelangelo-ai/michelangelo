import { type UserContextType, UserTimeZone } from '#core/providers/user-provider/types';
import { UserProvider } from '#core/providers/user-provider/user-provider';
import { WrapperComponentProps } from './types';

/**
 * Creates a React wrapper for testing components that use service features.
 * This wrapper is essential for testing components that use service hooks
 * like useStudioQuery, useStudioMutation, etc.
 *
 * @param serviceProvider - The service provider to use for the service context
 * @returns A wrapper component that provides service context to its children
 *
 * @example
 * ```tsx
 * // Simple usage with a specific route
 * const mockRequest = vi.fn();
 * const wrapper = getServiceProviderWrapper({ request: mockRequest });
 * render(<MyComponent />, { wrapper });
 *
 * expect(mockRequest).toHaveBeenCalledWith('requestId', { queryKey: ['requestId'] });
 * ```
 */
export function getUserProviderWrapper(userProvider?: Partial<UserContextType>) {
  const base = {
    timeZone: UserTimeZone.UTC,
  };

  return function ServiceProviderWrapper({ children }: WrapperComponentProps) {
    return (
      <UserProvider {...base} {...userProvider}>
        {children}
      </UserProvider>
    );
  };
}
