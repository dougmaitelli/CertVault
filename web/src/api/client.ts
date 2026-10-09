type Problem = {
  detail?: string;
};

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

const authenticationExpiredListeners = new Set<() => void>();

export function onAuthenticationExpired(listener: () => void): () => void {
  authenticationExpiredListeners.add(listener);
  return () => {
    authenticationExpiredListeners.delete(listener);
  };
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1/${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  if (!response.ok) {
    if (response.status === 401 && !init?.signal?.aborted) {
      authenticationExpiredListeners.forEach((listener) => listener());
    }
    const payload: unknown = await response.json().catch(() => undefined);
    const problem = payload as Problem | undefined;
    throw new APIError(response.status, problem?.detail ?? response.statusText);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const payload: unknown = await response.json();
  return payload as T;
}
