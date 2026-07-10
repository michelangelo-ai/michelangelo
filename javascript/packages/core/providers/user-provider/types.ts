import type { TimeZone } from '#core/types/time-types';

export enum UserRole {
  Admin = 'admin',
  Viewer = 'viewer',
}

export type UserContextType = {
  name?: string;
  email?: string;
  avatarUrl?: string;
  role?: UserRole;
  timeZone: TimeZone;
};
