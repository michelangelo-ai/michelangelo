import { useLocation } from 'react-router-dom-v5-compat';

export function useURLQueryString<T extends Record<string, string>>(): Partial<T> {
  const location = useLocation();
  const { search = '' } = location ?? {};
  return Object.fromEntries(new URLSearchParams(search)) as Partial<T>;
}
