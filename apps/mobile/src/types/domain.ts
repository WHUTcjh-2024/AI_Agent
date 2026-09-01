export type User = {
  id: string;
  nickname: string;
  avatarUrl?: string;
  schoolName: string;
};

export type MessageRole = 'user' | 'assistant';

export type Message = {
  id: string;
  sessionId: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  sourceIds?: string[];
  status?: 'streaming' | 'completed' | 'failed' | 'stopped';
};

export type Session = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  messages: Message[];
};

export type Source = {
  id: string;
  title: string;
  publisher: string;
  publishedAt: string;
  audience: string;
  summary: string;
  url: string;
  official: boolean;
};

export type Citation = {
  id: string;
  messageId: string;
  sourceId: string;
  excerpt: string;
};

export type Feedback = {
  id: string;
  messageId: string;
  value: 'helpful' | 'unhelpful';
  createdAt: string;
};

export type AgentRunStatus = 'created' | 'retrieving' | 'generating' | 'completed' | 'failed' | 'cancelled';

export type AgentRun = {
  id: string;
  sessionId: string;
  status: AgentRunStatus;
  createdAt: string;
};

export type ChatEvent =
  | { type: 'run_started'; run: AgentRun; sessionId: string }
  | { type: 'retrieval_started' }
  | { type: 'sources_updated'; sources: Source[] }
  | { type: 'message_delta'; messageId: string; delta: string }
  | { type: 'message_completed'; message: Message }
  | { type: 'run_failed'; error: string; retryable: boolean };

export type ChatScenario = 'normal' | 'multi_source' | 'no_reliable_source' | 'long_answer' | 'network_error';
