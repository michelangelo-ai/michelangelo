/**
 * The user's preferred timezone context for displaying time-based data.
 *
 * - `Local` — times are shown in the user's browser/system timezone
 * - `UTC` — times are shown in Coordinated Universal Time, independent of location
 */
export enum TimeZone {
  Local = 'local',
  UTC = 'utc',
}
