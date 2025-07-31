/**
 * Safely sets an item in localStorage with automatic JSON serialization.
 * Silently handles localStorage errors
 *
 * @param key - The localStorage key
 * @param item - The value to store (will be JSON.stringify'd)
 */
export function safeLocalStorageSetItem<T>(key: string, item: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(item));
  } catch {
    // Ignore localStorage errors (quota exceeded, private browsing, etc.)
  }
}

/**
 * Safely retrieves and deserializes an item from localStorage.
 * Returns the defaultValue on any error
 *
 * @param key - The localStorage key to retrieve
 * @param defaultValue - Value returned if retrieval/parsing fails
 * @returns The parsed value or defaultValue
 */
export function safeLocalStorageGetItem<T>(key: string, defaultValue: T): T {
  try {
    const item = localStorage.getItem(key);
    if (item === null) {
      return defaultValue;
    }
    const result = JSON.parse(item) as T;
    return result ?? defaultValue;
  } catch {
    return defaultValue;
  }
}
