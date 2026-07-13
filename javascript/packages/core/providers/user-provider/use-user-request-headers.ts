import { useUserProvider } from './use-user-provider';

export const useUserRequestHeaders = (): Record<string, string> => {
  const { name, email } = useUserProvider();

  const headers: Record<string, string> = {};
  if (email !== undefined) headers['x-user-email'] = email;
  if (name !== undefined) headers['x-user-name'] = name;

  return headers;
};
