import type { Source } from '../types/domain';

export const mockSources: Source[] = [
  {
    id: 'source_transfer_2026',
    title: '关于 2026 年本科生转专业工作的通知',
    publisher: '武汉理工大学本科生院',
    publishedAt: '2026-04-30T08:00:00.000Z',
    audience: '全日制本科生',
    summary: '学校启动 2026 年本科生转专业工作，明确申请时间、基本条件、学院实施方案与结果公示安排。',
    url: 'https://jwc.whut.edu.cn/',
    official: true,
  },
  {
    id: 'source_transfer_policy',
    title: '武汉理工大学普通全日制本科生转专业管理办法',
    publisher: '武汉理工大学教务处',
    publishedAt: '2025-09-01T08:00:00.000Z',
    audience: '全日制本科生',
    summary: '说明转专业申请资格、审核程序、学籍与课程衔接等长期制度要求。',
    url: 'https://jwc.whut.edu.cn/',
    official: true,
  },
  {
    id: 'source_transfer_school',
    title: '材料学院 2026 年接收转专业学生工作方案',
    publisher: '武汉理工大学材料科学与工程学院',
    publishedAt: '2026-05-03T08:00:00.000Z',
    audience: '申请转入材料学院的本科生',
    summary: '明确接收名额、课程要求、考核方式与材料提交方式。',
    url: 'https://smse.whut.edu.cn/',
    official: true,
  },
  {
    id: 'source_calendar',
    title: '武汉理工大学 2026—2027 学年校历',
    publisher: '武汉理工大学校长办公室',
    publishedAt: '2026-06-18T08:00:00.000Z',
    audience: '全校师生',
    summary: '包含开学、教学周、考试周和主要节假日安排。',
    url: 'https://www.whut.edu.cn/',
    official: true,
  },
  {
    id: 'source_library',
    title: '图书馆开放时间与服务指南（2026—2027 学年）',
    publisher: '武汉理工大学图书馆',
    publishedAt: '2026-08-25T08:00:00.000Z',
    audience: '全校师生',
    summary: '说明马房山校区、南湖校区各馆区开放时间及特殊日期安排。',
    url: 'https://lib.whut.edu.cn/',
    official: true,
  },
];

export const getMockSourceById = (id: string): Source | undefined => mockSources.find((source) => source.id === id);
