import AsyncStorage from '@react-native-async-storage/async-storage';

import { chunkAnswer, getScenarioDefinition, resolveScenario } from '../../mocks/scenarios';
import { getMockSourceById } from '../../mocks/sources';
import type { AgentRun, ChatEvent, Feedback, Message, Session, Source } from '../../types/domain';
import { createId } from '../../utils/id';
import type { ChatService, StreamMessageRequest } from './ChatService';

const HISTORY_KEY = 'asku.mock.sessions.v1';
const FEEDBACK_KEY = 'asku.mock.feedback.v1';

function wait(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new Error('aborted'));
      return;
    }

    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        reject(new Error('aborted'));
      },
      { once: true },
    );
  });
}

function isoHoursAgo(hours: number): string {
  return new Date(Date.now() - hours * 3_600_000).toISOString();
}

function seedSessions(): Session[] {
  const firstSessionId = 'session_seed_transfer';
  const secondSessionId = 'session_seed_cet';
  return [
    {
      id: firstSessionId,
      title: '今年转专业什么时候开始？',
      createdAt: isoHoursAgo(2),
      updatedAt: isoHoursAgo(2),
      messages: [
        {
          id: 'message_seed_user_transfer',
          sessionId: firstSessionId,
          role: 'user',
          content: '今年转专业什么时候开始？',
          createdAt: isoHoursAgo(2),
          status: 'completed',
        },
        {
          id: 'message_seed_assistant_transfer',
          sessionId: firstSessionId,
          role: 'assistant',
          content: getScenarioDefinition('转专业').answer,
          createdAt: isoHoursAgo(2),
          sourceIds: ['source_transfer_2026', 'source_transfer_policy', 'source_transfer_school'],
          status: 'completed',
        },
      ],
    },
    {
      id: secondSessionId,
      title: '四六级什么时候报名？',
      createdAt: isoHoursAgo(26),
      updatedAt: isoHoursAgo(26),
      messages: [
        {
          id: 'message_seed_user_cet',
          sessionId: secondSessionId,
          role: 'user',
          content: '四六级什么时候报名？',
          createdAt: isoHoursAgo(26),
          status: 'completed',
        },
      ],
    },
  ];
}

export class MockChatService implements ChatService {
  private async readSessions(): Promise<Session[]> {
    const raw = await AsyncStorage.getItem(HISTORY_KEY);
    if (!raw) {
      const seeded = seedSessions();
      await this.writeSessions(seeded);
      return seeded;
    }
    return JSON.parse(raw) as Session[];
  }

  private async writeSessions(sessions: Session[]): Promise<void> {
    await AsyncStorage.setItem(HISTORY_KEY, JSON.stringify(sessions));
  }

  async createSession(initialTitle: string): Promise<Session> {
    const now = new Date().toISOString();
    const session: Session = {
      id: createId('session'),
      title: initialTitle.trim().slice(0, 28) || '新对话',
      createdAt: now,
      updatedAt: now,
      messages: [],
    };
    const sessions = await this.readSessions();
    await this.writeSessions([session, ...sessions]);
    return session;
  }

  async *streamMessage(request: StreamMessageRequest, signal?: AbortSignal): AsyncIterable<ChatEvent> {
    const sessions = await this.readSessions();
    const session = sessions.find((item) => item.id === request.sessionId);
    if (!session) {
      yield { type: 'run_failed', error: '当前对话不存在，请返回后重新提问。', retryable: false };
      return;
    }

    const userMessage: Message = {
      id: request.userMessageId,
      sessionId: request.sessionId,
      role: 'user',
      content: request.question,
      createdAt: new Date().toISOString(),
      status: 'completed',
    };
    session.messages.push(userMessage);
    session.updatedAt = userMessage.createdAt;
    await this.writeSessions(sessions);

    const run: AgentRun = {
      id: createId('run'),
      sessionId: request.sessionId,
      status: 'created',
      createdAt: new Date().toISOString(),
    };
    yield { type: 'run_started', run, sessionId: request.sessionId };

    try {
      await wait(420, signal);
      yield { type: 'retrieval_started' };

      const scenario = resolveScenario(request.question);
      if (scenario === 'network_error') {
        await wait(720, signal);
        yield { type: 'run_failed', error: '网络连接失败，请检查网络后重新尝试。', retryable: true };
        return;
      }

      const definition = getScenarioDefinition(request.question);
      await wait(680, signal);
      yield { type: 'sources_updated', sources: definition.sources };
      await wait(520, signal);

      const answer =
        scenario === 'no_reliable_source'
          ? '暂时没有找到可靠的学校官方信息。\n\n你可以换一种方式提问，或查看学校相关部门发布的原始资料。'
          : definition.answer;
      const messageId = createId('message');
      for (const delta of chunkAnswer(answer)) {
        await wait(definition.chunkDelayMs, signal);
        yield { type: 'message_delta', messageId, delta };
      }

      const completedMessage: Message = {
        id: messageId,
        sessionId: request.sessionId,
        role: 'assistant',
        content: answer,
        createdAt: new Date().toISOString(),
        sourceIds: definition.sources.map((source) => source.id),
        status: 'completed',
      };
      session.messages.push(completedMessage);
      session.updatedAt = completedMessage.createdAt;
      await this.writeSessions(sessions);
      yield { type: 'message_completed', message: completedMessage };
    } catch (error) {
      if (signal?.aborted) return;
      yield { type: 'run_failed', error: error instanceof Error ? error.message : '生成失败，请重试。', retryable: true };
    }
  }

  async getHistory(): Promise<Session[]> {
    const sessions = await this.readSessions();
    return [...sessions].sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  }

  async getSession(sessionId: string): Promise<Session | null> {
    const sessions = await this.readSessions();
    return sessions.find((item) => item.id === sessionId) ?? null;
  }

  async deleteSession(sessionId: string): Promise<void> {
    const sessions = await this.readSessions();
    await this.writeSessions(sessions.filter((item) => item.id !== sessionId));
  }

  async clearHistory(): Promise<void> {
    await this.writeSessions([]);
  }

  async resetDemoHistory(): Promise<void> {
    await this.writeSessions(seedSessions());
  }

  async getSource(sourceId: string): Promise<Source | null> {
    await wait(160);
    return getMockSourceById(sourceId) ?? null;
  }

  async submitFeedback(messageId: string, value: Feedback['value']): Promise<Feedback> {
    const feedback: Feedback = { id: createId('feedback'), messageId, value, createdAt: new Date().toISOString() };
    const raw = await AsyncStorage.getItem(FEEDBACK_KEY);
    const records = raw ? (JSON.parse(raw) as Feedback[]) : [];
    await AsyncStorage.setItem(FEEDBACK_KEY, JSON.stringify([feedback, ...records]));
    await wait(180);
    return feedback;
  }

  copyableAnswer(message: Message): string {
    return `${message.content}\n\n具体安排请以学校最终发布的正式通知为准。`;
  }
}
