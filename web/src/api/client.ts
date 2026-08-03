type Problem = {
  detail?: string;
};

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1/${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => undefined);
    const problem = payload as Problem | undefined;
    throw new Error(problem?.detail ?? response.statusText);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const payload: unknown = await response.json();
  return payload as T;
}
