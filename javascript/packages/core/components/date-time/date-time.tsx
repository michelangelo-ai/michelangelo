import { useUserProvider } from '#core/providers/user-provider/use-user-provider';
import { timestampToString } from '#core/utils/time-utils';

import type { Props } from './types';

/**
 * Displays timestamps in a user-friendly format using the current user's timezone.
 *
 * This component automatically converts UTC timestamps to the user's preferred timezone
 * (from UserProvider) and formats them for display. Handles null/undefined timestamps
 * gracefully by rendering nothing.
 *
 * Features:
 * - Automatic timezone conversion based on user preferences
 * - Consistent date/time formatting across the application
 * - Null-safe rendering
 * - Integration with UserProvider
 *
 * @param props.timestamp - ISO 8601 timestamp string or null/undefined. Expected format:
 *   "2024-01-15T10:30:00Z" or similar UTC timestamp.
 *
 * @example
 * ```tsx
 * // Display pipeline creation time
 * <DateTime timestamp="2024-01-15T10:30:00Z" />
 * // Output (PST user): "Jan 15, 2024, 2:30 AM"
 *
 * // In table cell
 * <td>
 *   <DateTime timestamp={pipeline.createdAt} />
 * </td>
 *
 * // Handles null gracefully
 * <DateTime timestamp={null} />
 * // Output: (nothing rendered)
 * ```
 */
export function DateTime(props: Props) {
  const { timestamp } = props;

  const { timeZone } = useUserProvider();

  const dateString = timestamp ? timestampToString(timestamp, timeZone) : null;

  return <>{dateString}</>;
}
