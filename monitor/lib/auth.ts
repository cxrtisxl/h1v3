const API_URL_KEY = "h1v3_api_url";
const API_KEY_KEY = "h1v3_api_key";

export function getApiUrl(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(API_URL_KEY);
}

export function getApiKey(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(API_KEY_KEY);
}

export function setAuth(apiUrl: string, apiKey: string) {
  localStorage.setItem(API_URL_KEY, apiUrl);
  localStorage.setItem(API_KEY_KEY, apiKey);
}

export function clearAuth() {
  localStorage.removeItem(API_URL_KEY);
  localStorage.removeItem(API_KEY_KEY);
}

export function isAuthenticated(): boolean {
  return !!getApiUrl();
}
