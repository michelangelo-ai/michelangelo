export type DevProfile = {
  name: string;
  email: string;
  avatarUrl: string;
};

export type GithubUserResponse = {
  name: string | null;
  email: string | null;
  avatar_url: string;
};
