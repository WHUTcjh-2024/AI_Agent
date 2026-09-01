import { useCallback, useEffect, useRef, useState } from 'react';

import { useServices } from '../services/ServiceProvider';
import { useAppStore } from '../store/AppStore';
import type { Feedback, Message, Source } from '../types/domain';
import { createId } from '../utils/id';
import type { AgentStage } from '../components/chat/AgentStatus';

type Options = { initialSessionId?: string; initialQuestion?: string };

export function useChatController({ initialSessionId, initialQuestion }: Options) {
  const { chat } = useServices();
  const { notifyHistoryChanged } = useAppStore();
  const [sessionId, setSessionId] = useState(initialSessionId);
  const [messages, setMessages] = useState<Message[]>([]);
  const [sources, setSources] = useState<Record<string, Source>>({});
  const [feedback, setFeedback] = useState<Record<string, Feedback['value']>>({});
  const [input, setInput] = useState('');
  const [isGenerating, setGenerating] = useState(false);
  const [stage, setStage] = useState<AgentStage | null>(null);
  const [activeAssistantId, setActiveAssistantId] = useState<string | null>(null);
  const [sourceCount, setSourceCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [lastQuestion, setLastQuestion] = useState('');
  const abortRef = useRef<AbortController | null>(null);
  const initialQuestionSent = useRef(false);

  useEffect(() => {
    if (!initialSessionId) return;
    let active = true;
    void (async () => {
      try {
        const session = await chat.getSession(initialSessionId);
        if (!active) return;
        if (!session) { setError('这条历史对话不存在。'); return; }
        setMessages(session.messages);
        const ids = Array.from(new Set(session.messages.flatMap((message) => message.sourceIds ?? [])));
        const loaded = await Promise.allSettled(ids.map((id) => chat.getSource(id)));
        if (!active) return;
        const available = loaded
          .filter((result): result is PromiseFulfilledResult<Source | null> => result.status === 'fulfilled')
          .map((result) => result.value)
          .filter((source): source is Source => Boolean(source));
        setSources(Object.fromEntries(available.map((source) => [source.id, source])));
      } catch {
        if (active) setError('这条历史对话暂时无法读取。');
      }
    })();
    return () => { active = false; };
  }, [chat, initialSessionId]);

  const stopGeneration = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setGenerating(false);
    setStage(null);
    setMessages((current) => current.map((message) => (
      message.id === activeAssistantId ? { ...message, status: 'stopped', content: message.content || '已停止生成。' } : message
    )));
    notifyHistoryChanged();
  }, [activeAssistantId, notifyHistoryChanged]);

  const sendQuestion = useCallback(async (rawQuestion: string) => {
    const question = rawQuestion.trim();
    if (!question || isGenerating) return;

    setInput('');
    setError(null);
    setStage(null);
    setActiveAssistantId(null);
    setSourceCount(0);
    setLastQuestion(question);
    setGenerating(true);

    let currentSessionId = sessionId;
    if (!currentSessionId) {
      try {
        const session = await chat.createSession(question);
        currentSessionId = session.id;
        setSessionId(session.id);
      } catch (creationError) {
        setError(creationError instanceof Error ? creationError.message : '创建对话失败，请检查服务连接。');
        setGenerating(false);
        return;
      }
    }

    const userMessageId = createId('message');
    setMessages((current) => [...current, {
      id: userMessageId, sessionId: currentSessionId, role: 'user', content: question,
      createdAt: new Date().toISOString(), status: 'completed',
    }]);

    const controller = new AbortController();
    abortRef.current = controller;
    try {
      for await (const event of chat.streamMessage({ sessionId: currentSessionId, question, userMessageId }, controller.signal)) {
        if (event.type === 'retrieval_started') {
          setStage('retrieving');
        } else if (event.type === 'sources_updated') {
          setSources((current) => ({ ...current, ...Object.fromEntries(event.sources.map((source) => [source.id, source])) }));
          setSourceCount(event.sources.length);
          setStage('sources');
        } else if (event.type === 'message_delta') {
          setStage('composing');
          setActiveAssistantId(event.messageId);
          setMessages((current) => {
            const existing = current.some((message) => message.id === event.messageId);
            if (existing) return current.map((message) => message.id === event.messageId ? { ...message, content: message.content + event.delta } : message);
            return [...current, { id: event.messageId, sessionId: currentSessionId, role: 'assistant', content: event.delta, createdAt: new Date().toISOString(), status: 'streaming' }];
          });
        } else if (event.type === 'message_completed') {
          setMessages((current) => current.some((message) => message.id === event.message.id)
            ? current.map((message) => message.id === event.message.id ? event.message : message)
            : [...current, event.message]);
          setActiveAssistantId(event.message.id);
          setStage('completed');
        } else if (event.type === 'run_failed') {
          setError(event.error);
          setStage(null);
        }
      }
    } catch (streamError) {
      if (!controller.signal.aborted) {
        setStage(null);
        setError(streamError instanceof Error ? streamError.message : '生成失败，请检查网络后重试。');
      }
    } finally {
      if (!controller.signal.aborted) {
        setGenerating(false);
        notifyHistoryChanged();
      }
      if (abortRef.current === controller) abortRef.current = null;
    }
  }, [chat, isGenerating, notifyHistoryChanged, sessionId]);

  const submitFeedback = useCallback(async (messageId: string, value: Feedback['value']) => {
    const previous = feedback[messageId];
    setFeedback((current) => ({ ...current, [messageId]: value }));
    try {
      await chat.submitFeedback(messageId, value);
    } catch (feedbackError) {
      setFeedback((current) => {
        const next = { ...current };
        if (previous) next[messageId] = previous; else delete next[messageId];
        return next;
      });
      throw feedbackError;
    }
  }, [chat, feedback]);

  useEffect(() => {
    if (!initialQuestion || initialQuestionSent.current) return;
    initialQuestionSent.current = true;
    const timer = setTimeout(() => { void sendQuestion(initialQuestion); }, 120);
    return () => clearTimeout(timer);
  }, [initialQuestion, sendQuestion]);

  useEffect(() => () => abortRef.current?.abort(), []);

  return {
    activeAssistantId, error, feedback, input, isGenerating, lastQuestion, messages,
    sendQuestion, setInput, sourceCount, sources, stage, stopGeneration, submitFeedback,
  };
}
