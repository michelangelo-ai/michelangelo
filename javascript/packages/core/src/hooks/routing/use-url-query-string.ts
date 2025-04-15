import { useLocation } from 'react-router';

export default function useURLQueryString<T extends Record<string, string>>(): Partial<T> {
  const location = useLocation();
  const { search = '' } = location ?? {};
  return Object.fromEntries(new URLSearchParams(search)) as Partial<T>;
}
