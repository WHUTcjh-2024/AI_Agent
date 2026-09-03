export class ApiError extends Error {
  constructor(message, status, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function request(path, { method = "GET", body, signal } = {}) {
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    cache: "no-store",
    signal,
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return null;
  let payload;
  try {
    payload = await response.json();
  } catch {
    throw new ApiError(
      "服务返回的数据格式不正确，请稍后重试。",
      response.status,
      "invalid_response",
    );
  }
  if (!response.ok)
    throw new ApiError(
      payload.error?.message || "请求失败，请稍后重试。",
      response.status,
      payload.error?.code,
    );
  return payload;
}
