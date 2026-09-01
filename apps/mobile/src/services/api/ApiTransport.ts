type ApiErrorBody = { error?: { code?: string; message?: string; requestId?: string }; message?: string };

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code = 'api_error',
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export class ApiTransport {
  constructor(readonly baseUrl: string) {}

  request<T>(path: string, init: RequestInit = {}, accessToken?: string): Promise<T> {
    return this.fetch(path, init, accessToken).then((response) => this.parse<T>(response));
  }

  fetch(path: string, init: RequestInit = {}, accessToken?: string): Promise<Response> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (init.body != null) headers['Content-Type'] = 'application/json';
    if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
    return globalThis.fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: { ...headers, ...init.headers },
    });
  }

  async parse<T>(response: Response): Promise<T> {
    if (response.ok) {
      if (response.status === 204) return undefined as T;
      return response.json() as Promise<T>;
    }
    let body: ApiErrorBody = {};
    try { body = (await response.json()) as ApiErrorBody; } catch { /* Preserve HTTP metadata. */ }
    throw new ApiError(
      body.error?.message ?? body.message ?? `请求失败（${response.status}）`,
      response.status,
      body.error?.code,
      body.error?.requestId,
    );
  }
}
