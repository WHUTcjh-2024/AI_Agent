import type { ChatScenario, Source } from '../types/domain';
import { mockSources } from './sources';

export const frequentlyAskedQuestions = [
  '转专业有什么要求？',
  '四六级什么时候报名？',
  '奖学金怎么评？',
  '图书馆几点关门？',
  '校历在哪里看？',
];

const transferAnswer = `根据武汉理工大学本科生院发布的最新通知，**2026 年本科生转专业工作预计于 5 月开始**。

## 申请时间

5 月 10 日 – 5 月 24 日

## 你需要注意

1. 不同学院可能有不同接收条件；
2. 申请前应查看目标学院实施方案；
3. 最终安排请以学校正式通知为准。

建议先确认自己的已修课程与成绩是否满足目标学院要求，再准备申请材料。`;

const normalAnswer = `根据目前可查询到的学校官方资料，你可以先查看对应部门发布的最新通知。

## 建议操作

- 优先确认通知的**发布时间**和**发布单位**；
- 如果存在多个版本，以标注为当前有效的正式文件为准；
- 涉及具体学院时，再查看学院实施方案。

我已经把相关官方来源整理在下方。`;

const libraryAnswer = `武汉理工大学图书馆当前开放安排以各馆区通知为准。

## 常规闭馆时间

- 马房山校区：22:00
- 南湖校区：21:30

节假日和考试周可能临时调整，出发前建议查看图书馆最新通知。`;

const longAnswer = `综合测评通常由**思想品德、课程学习、实践创新与社会工作**等部分组成，不同学院的细则和权重可能不同。

## 先确认适用文件

1. 查看本学院本学年综合测评实施细则；
2. 确认适用年级和专业；
3. 对照材料提交时间与证明要求。

## 常见准备材料

- 竞赛、科研和创新创业证明；
- 志愿服务与社会工作记录；
- 文体活动、荣誉和表彰材料；
- 学院要求的其他原始证明。

## 容易忽略的事项

- 同一成果通常不能重复加分；
- 逾期材料可能不予补交；
- 公示期内应及时核对个人结果；
- 学院细则与学校原则冲突时，应向辅导员或学院负责部门确认。

最终分值与评定结果请以学院正式公示为准。`;

export type MockScenarioDefinition = {
  answer: string;
  sources: Source[];
  chunkDelayMs: number;
};

export function resolveScenario(question: string): ChatScenario {
  const normalized = question.trim().toLowerCase();
  if (normalized.includes('网络错误') || normalized.includes('offline')) return 'network_error';
  if (normalized.includes('养宠物') || normalized.includes('可靠来源') || normalized.includes('no-source')) return 'no_reliable_source';
  if (normalized.includes('综测') || normalized.includes('长回答')) return 'long_answer';
  if (normalized.includes('转专业')) return 'multi_source';
  return 'normal';
}

export function getScenarioDefinition(question: string): MockScenarioDefinition {
  const scenario = resolveScenario(question);
  if (scenario === 'multi_source') return { answer: transferAnswer, sources: mockSources.slice(0, 3), chunkDelayMs: 210 };
  if (scenario === 'long_answer') return { answer: longAnswer, sources: [mockSources[1]], chunkDelayMs: 170 };
  if (question.includes('图书馆')) return { answer: libraryAnswer, sources: [mockSources[4]], chunkDelayMs: 190 };
  if (question.includes('校历')) return { answer: normalAnswer, sources: [mockSources[3]], chunkDelayMs: 190 };
  if (scenario === 'no_reliable_source') return { answer: '', sources: [], chunkDelayMs: 180 };
  return { answer: normalAnswer, sources: [mockSources[0]], chunkDelayMs: 190 };
}

export function chunkAnswer(answer: string): string[] {
  const chunks = answer.split(/(?<=。|；|！|？|\n\n)/u).filter(Boolean);
  return chunks.length > 1 ? chunks : [answer];
}
