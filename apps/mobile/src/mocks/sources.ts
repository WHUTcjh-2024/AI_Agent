import type { Citation, Source } from '../types/domain';

export const mockSources: Source[] = [
  {
    id: 'source_transfer_2026',
    title: '关于 2026 年本科生转专业工作的通知',
    publisher: '武汉理工大学本科生院',
    department: '本科生院',
    publishedAt: '2026-04-30T08:00:00.000Z',
    audience: '全日制本科生',
    summary: '学校启动 2026 年本科生转专业工作，明确申请时间、基本条件、学院实施方案与结果公示安排。',
    url: 'https://jwc.whut.edu.cn/',
    officialUrl: 'https://jwc.whut.edu.cn/',
    official: true,
    sourceType: 'OFFICIAL_WEB', documentType: 'HTML', authority: 'OFFICIAL_DEPARTMENT', freshness: 'CURRENT',
    knowledgeBundleId: 'bundle_transfer_2026', attachments: [],
    evidence: ['学校启动 2026 年本科生转专业工作，并明确申请时间与学院实施方案要求。'],
  },
  {
    id: 'source_transfer_policy',
    title: '武汉理工大学普通全日制本科生转专业管理办法',
    publisher: '武汉理工大学教务处',
    department: '教务处',
    publishedAt: '2025-09-01T08:00:00.000Z',
    audience: '全日制本科生',
    summary: '说明转专业申请资格、审核程序、学籍与课程衔接等长期制度要求。',
    url: 'https://jwc.whut.edu.cn/',
    officialUrl: 'https://jwc.whut.edu.cn/',
    official: true,
    sourceType: 'OFFICIAL_POLICY', documentType: 'HTML', authority: 'OFFICIAL_DEPARTMENT', freshness: 'CURRENT',
    knowledgeBundleId: 'bundle_transfer_2026', attachments: [],
    evidence: ['管理办法明确了转专业申请资格、审核程序与学籍衔接要求。'],
  },
  {
    id: 'source_transfer_school',
    title: '材料学院 2026 年接收转专业学生工作方案',
    publisher: '武汉理工大学材料科学与工程学院',
    department: '材料科学与工程学院',
    publishedAt: '2026-05-03T08:00:00.000Z',
    audience: '申请转入材料学院的本科生',
    summary: '明确接收名额、课程要求、考核方式与材料提交方式。',
    url: 'https://smse.whut.edu.cn/',
    officialUrl: 'https://smse.whut.edu.cn/',
    official: true,
    sourceType: 'OFFICIAL_SCHOOL', documentType: 'PDF', authority: 'OFFICIAL_SCHOOL', freshness: 'CURRENT',
    knowledgeBundleId: 'bundle_transfer_2026',
    attachmentUrl: 'https://smse.whut.edu.cn/', parentPageUrl: 'https://smse.whut.edu.cn/',
    attachments: [{ id: 'attachment_transfer_plan', name: '2026 年接收转专业学生工作方案.pdf', url: 'https://smse.whut.edu.cn/', documentType: 'PDF', parentPageUrl: 'https://smse.whut.edu.cn/' }],
    evidence: ['学院方案明确接收名额、课程要求、考核方式和材料提交方式。'],
  },
  {
    id: 'source_calendar',
    title: '武汉理工大学 2026—2027 学年校历',
    publisher: '武汉理工大学校长办公室',
    department: '校长办公室',
    publishedAt: '2026-06-18T08:00:00.000Z',
    audience: '全校师生',
    summary: '包含开学、教学周、考试周和主要节假日安排。',
    url: 'https://www.whut.edu.cn/',
    officialUrl: 'https://www.whut.edu.cn/',
    official: true,
    sourceType: 'OFFICIAL_ATTACHMENT', documentType: 'PDF', authority: 'OFFICIAL_DEPARTMENT', freshness: 'CURRENT',
    attachments: [{ id: 'attachment_calendar', name: '2026—2027 学年校历.pdf', url: 'https://www.whut.edu.cn/', documentType: 'PDF' }],
    evidence: ['校历列明开学、教学周、考试周和主要节假日安排。'],
  },
  {
    id: 'source_library',
    title: '图书馆开放时间与服务指南（2026—2027 学年）',
    publisher: '武汉理工大学图书馆',
    department: '图书馆',
    publishedAt: '2026-08-25T08:00:00.000Z',
    audience: '全校师生',
    summary: '说明马房山校区、南湖校区各馆区开放时间及特殊日期安排。',
    url: 'https://lib.whut.edu.cn/',
    officialUrl: 'https://lib.whut.edu.cn/',
    official: true,
    sourceType: 'OFFICIAL_WEB', documentType: 'HTML', authority: 'OFFICIAL_DEPARTMENT', freshness: 'CURRENT',
    attachments: [], evidence: ['马房山校区与南湖校区馆区开放时间以图书馆最新通知为准。'],
  },
];

export const getMockSourceById = (id: string): Source | undefined => mockSources.find((source) => source.id === id);

export function createMockCitations(sources: Source[]): Citation[] {
	const citations: Citation[] = [];
	sources.forEach((source) => {
    const evidenceText = source.evidence[0];
    if (!evidenceText || !(source.attachmentUrl || source.officialUrl || source.parentPageUrl || source.url)) return;
    citations.push({
      citationId: `citation_${source.id}`,
      index: citations.length + 1,
      sourceId: source.id,
      askuDocumentId: `document_${source.id}`,
      weknoraKnowledgeId: `knowledge_${source.id}`,
      chunkId: `chunk_${source.id}`,
      title: source.title,
      sourceName: source.publisher,
      department: source.department,
      publishDate: source.publishedAt,
      sourceType: source.sourceType,
      documentType: source.documentType,
      officialUrl: source.officialUrl || source.url,
      attachmentUrl: source.attachmentUrl,
      parentPageUrl: source.parentPageUrl,
      evidenceText,
      authority: source.authority || 'OFFICIAL_DEPARTMENT',
      freshness: source.freshness,
      knowledgeBundleId: source.knowledgeBundleId,
    } satisfies Citation);
  });
	return citations;
}
