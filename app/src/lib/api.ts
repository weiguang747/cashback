/**
 * Client for the cashback server. Set EXPO_PUBLIC_API_URL in `.env` to point
 * somewhere other than localhost (see `.env.example`).
 */
export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

export type User = { email: string };
export type SignInResponse = { token: string; user: User };

/** Signs in with an email. The server accepts any valid email for now. */
export function signIn(email: string): Promise<SignInResponse> {
  return post<SignInResponse>('/auth/sign-in', { email });
}

async function post<T>(path: string, body: unknown): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  } catch {
    throw new Error("Can't reach the server. Check your connection and try again.");
  }

  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new Error(data?.error ?? 'Something went wrong. Please try again.');
  }
  return data as T;
}
