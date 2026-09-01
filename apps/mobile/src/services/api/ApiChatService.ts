import type { AgentRun, ChatEvent, Feedback, Message, Session, Source } from '../../types/domain';
import { createId } from '../../utils/id';
import type { ChatService, StreamMessageRequest } from '../chat/ChatService';
import { ApiClient } from './ApiClient';
import { ApiError } from './ApiTransport';

type SendMessageResponse = { run: AgentRun; userMessage: Message };
type SessionsResponse = { sessions: Session[] };
type WireEvent = { id: number; type: string; payload: unknown };

export class ApiChatService implements ChatService {
  constructor(private readonly client: ApiClient) {}

  createSession(initialTitle: string): Promise<Session> {
    return this.client.request<Session>('/v1/sessions', {
      method: 'POST', body: JSON.stringify({ title: initialTitle }),
    });
  }

  async *streamMessage(request: StreamMessageRequest, signal?: AbortSignal): AsyncIterable<ChatEvent> {
    let runId: string | null = null;
    try {
      const accepted = await this.client.request<SendMessageResponse>(
        `/v1/sessions/${encodeURIComponent(request.sessionId)}/messages`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': `${request.userMessageId}:${createId('request')}` },
          body: JSON.stringify({ question: request.question, userMessageId: request.userMessageId }),
          signal,
        },
      );
      runId = accepted.run.id;

      let lastEventId = 0;
      for (let attempt = 0; attempt < 3; attempt += 1) {
        try {
          for await (const wireEvent of this.openEventStream(runId, lastEventId, signal)) {
            if (wireEvent.id <= lastEventId) continue;
            lastEventId = wireEvent.id;
            const event = mapWireEvent(wireEvent);
            if (event) yield event;
            if (wireEvent.type === 'run.completed' || wireEvent.type === 'run.failed') return;
          }
          throw new Error('SSE connection ended before a terminal event');
        } catch (error) {
          if (signal?.aborted) return;
          if (attempt === 2) throw error;
          await retryDelay(250 * (attempt + 1), signal);
        }
      }
    } catch (error) {
      if (signal?.aborted) return;
      yield {
        type: 'run_failed',
        error: error instanceof Error ? error.message : '连接 AskU 服务失败，请稍后重试。',
        retryable: !(error instanceof ApiError && error.status >= 400 && error.status < 500),
      };
    } finally {
      if (signal?.aborted && runId) {
        void this.client.request<void>(`/v1/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' }).catch(() => undefined);
      }
    }
  }

  async getHistory(): Promise<Session[]> {
    return (await this.client.request<SessionsResponse>('/v1/sessions')).sessions;
  }

  async getSession(sessionId: string): Promise<Session | null> {
    try { return await this.client.request<Session>(`/v1/sessions/${encodeURIComponent(sessionId)}`); }
    catch (error) {
      if (error instanceof ApiError && error.status === 404) return null;
      throw error;
    }
  }

  deleteSession(sessionId: string): Promise<void> {
    return this.client.request<void>(`/v1/sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE' });
  }

  clearHistory(): Promise<void> {
    return this.client.request<void>('/v1/sessions', { method: 'DELETE' });
  }

  resetDemoHistory(): Promise<void> {
    return this.client.request<void>('/v1/dev/seed', { method: 'POST', body: '{}' });
  }

  async getSource(sourceId: string): Promise<Source | null> {
    try { return await this.client.request<Source>(`/v1/sources/${encodeURIComponent(sourceId)}`); }
    catch (error) {
      if (error instanceof ApiError && error.status === 404) return null;
      throw error;
    }
  }

  submitFeedback(messageId: string, value: Feedback['value']): Promise<Feedback> {
    return this.client.request<Feedback>('/v1/feedback', {
      method: 'POST', body: JSON.stringify({ messageId, value }),
    });
  }

  copyableAnswer(message: Message): string {
    return `${message.content}\n\n具体安排请以学校最终发布的正式通知为准。`;
  }

  private async *openEventStream(runId: string, after: number, signal?: AbortSignal): AsyncIterable<WireEvent> {
    const response = await this.client.authenticatedFetch(
      `/v1/runs/${encodeURIComponent(runId)}/events?after=${after}`,
      { headers: { Accept: 'text/event-stream' }, signal },
    );
    if (!response.ok) await this.client.parse<never>(response);

    if (!response.body) {
      yield* parseSseText(await response.text());
      return;
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, '\n');
      let boundary = buffer.indexOf('\n\n');
      while (boundary >= 0) {
        const parsed = parseSseBlock(buffer.slice(0, boundary));
        buffer = buffer.slice(boundary + 2);
        if (parsed) yield parsed;
        boundary = buffer.indexOf('\n\n');
      }
      if (done) break;
    }
    const tail = parseSseBlock(buffer);
    if (tail) yield tail;
  }
}

function retryDelay(durationMs: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, durationMs);
    signal?.addEventListener('abort', () => { clearTimeout(timer); resolve(); }, { once: true });
  });
}

function* parseSseText(text: string): Iterable<WireEvent> {
  for (const block of text.replace(/\r\n/g, '\n').split('\n\n')) {
    const event = parseSseBlock(block);
    if (event) yield event;
  }
}

export function parseSseBlock(block: string): WireEvent | null {
  let id = 0;
  let type = '';
  const data: string[] = [];
  for (const line of block.split('\n')) {
    if (!line || line.startsWith(':')) continue;
    if (line.startsWith('id:')) id = Number(line.slice(3).trim());
    else if (line.startsWith('event:')) type = line.slice(6).trim();
    else if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
  }
  if (!id || !type || !data.length) return null;
  try {
    return { id, type, payload: JSON.parse(data.join('\n')) as unknown };
  } catch {
    return null;
  }
}

export function mapWireEvent(event: WireEvent): ChatEvent | null {
  const payload = event.payload as Record<string, unknown>;
  switch (event.type) {
    case 'run.started': return { type: 'run_started', run: payload.run as AgentRun, sessionId: payload.sessionId as string };
    case 'retrieval.started': return { type: 'retrieval_started' };
    case 'sources.updated': return { type: 'sources_updated', sources: (payload.sources as Source[] | null) ?? [] };
    case 'message.delta': return { type: 'message_delta', messageId: payload.messageId as string, delta: payload.delta as string };
    case 'message.completed': return { type: 'message_completed', message: payload.message as Message };
    case 'run.failed': return { type: 'run_failed', error: payload.error as string, retryable: Boolean(payload.retryable) };
    default: return null;
  }
}
