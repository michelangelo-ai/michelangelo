export enum UserTimeZone {
  Local = 'local',
  UTC = 'utc',
}

export type UserContextType = {
  timeZone: UserTimeZone;
};
