// Same rules as backend/internal/domain/requests.go, so you get feedback while
// typing. The API still re-checks everything; the browser is never the boss.

const NAME_RE = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const IMAGE_RE = /^[a-zA-Z0-9][a-zA-Z0-9._/:@-]*$/;

export function validateName(value: string, maxLength: number): string | null {
  if (!value) return "Name is required.";
  if (value.length > maxLength) return `Name must be at most ${maxLength} characters.`;
  if (!NAME_RE.test(value)) return "Use lowercase letters, digits and '-', starting and ending with a letter or digit.";
  return null;
}

export function validateImage(value: string): string | null {
  if (!value) return "Image is required.";
  if (value.length > 255) return "Image must be at most 255 characters.";
  if (!IMAGE_RE.test(value)) return "Use an image reference such as nginx:stable or ghcr.io/org/app:1.2.";
  return null;
}

export function validateReplicas(value: string, max = 50): string | null {
  if (value.trim() === "") return "Replicas is required.";
  const n = Number(value);
  if (!Number.isInteger(n) || n < 0 || n > max) return `Replicas must be a whole number from 0 to ${max}.`;
  return null;
}
