import type { ChatEvent, Feedback, Message, Session, Source } from '../../types/domain';

export type StreamMessageRequest = {
  sessionId: string;
  question: string;
  userMessageId: string;
};

export interface ChatService {
  createSession(initialTitle: string): Promise<Session>;
  streamMessage(request: StreamMessageRequest, signal?: AbortSignal): AsyncIterable<ChatEvent>;
  getHistory(): Promise<Session[]>;
  getSession(sessionId: string): Promise<Session | null>;
  deleteSession(sessionId: string): Promise<void>;
  clearHistory(): Promise<void>;
  resetDemoHistory(): Promise<void>;
  getSource(sourceId: string): Promise<Source | null>;
  submitFeedback(messageId: string, value: Feedback['value']): Promise<Feedback>;
  copyableAnswer(message: Message): string;
}
