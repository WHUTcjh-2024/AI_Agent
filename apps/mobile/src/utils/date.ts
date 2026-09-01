export function formatDateLabel(isoDate: string): string {
  const date = new Date(isoDate);
  const today = new Date();
  const startOfToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
  const startOfDate = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const dayDiff = Math.round((startOfToday - startOfDate) / 86_400_000);

  if (dayDiff === 0) return '今天';
  if (dayDiff === 1) return '昨天';
  return `${date.getMonth() + 1} 月 ${date.getDate()} 日`;
}

export function formatPublishedDate(isoDate: string): string {
  const date = new Date(isoDate);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 1970) return '发布时间未标注';
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}
