export type AccountResource = {
  email: string;
  userId: string;
  referralCode: string;
  invitedBy: string;
  createdAt: number;
  passwordChangedAt: number;
  isAdmin: boolean;
  requiresReauthentication: boolean;
  csrf: string;
};

function isFiniteNonNegative(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

export function isAccountResource(value: unknown): value is AccountResource {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  return (
    typeof item.email === "string" && item.email.length <= 254 && item.email.includes("@") &&
    typeof item.userId === "string" && item.userId.length > 0 && item.userId.length <= 200 &&
    typeof item.referralCode === "string" && item.referralCode.length <= 32 &&
    typeof item.invitedBy === "string" && item.invitedBy.length <= 64 &&
    isFiniteNonNegative(item.createdAt) && isFiniteNonNegative(item.passwordChangedAt) &&
    typeof item.isAdmin === "boolean" && typeof item.requiresReauthentication === "boolean" &&
    typeof item.csrf === "string" && /^[A-Za-z0-9_-]{24,128}$/.test(item.csrf)
  );
}
