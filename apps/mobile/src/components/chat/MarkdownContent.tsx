import { Fragment, type ReactNode, useMemo } from 'react';
import { Linking, StyleSheet, Text, View } from 'react-native';

import { colors, spacing, typography } from '../../theme';

type Block =
  | { type: 'heading'; level: number; content: string }
  | { type: 'paragraph'; content: string }
  | { type: 'bullet'; items: string[] }
  | { type: 'numbered'; items: string[] };

function parseBlocks(markdown: string): Block[] {
  const blocks: Block[] = [];
  const lines = markdown.replace(/\r\n/g, '\n').split('\n');
  let paragraph: string[] = [];
  let list: { type: 'bullet' | 'numbered'; items: string[] } | null = null;

  const flushParagraph = () => {
    if (paragraph.length) blocks.push({ type: 'paragraph', content: paragraph.join(' ') });
    paragraph = [];
  };
  const flushList = () => {
    if (list) blocks.push(list);
    list = null;
  };

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) {
      flushParagraph();
      flushList();
      continue;
    }
    const heading = /^(#{1,3})\s+(.+)$/.exec(trimmed);
    if (heading) {
      flushParagraph();
      flushList();
      blocks.push({ type: 'heading', level: heading[1].length, content: heading[2] });
      continue;
    }
    const bullet = /^[-*]\s+(.+)$/.exec(trimmed);
    if (bullet) {
      flushParagraph();
      if (list?.type !== 'bullet') {
        flushList();
        list = { type: 'bullet', items: [] };
      }
      list.items.push(bullet[1]);
      continue;
    }
    const numbered = /^\d+[.)]\s+(.+)$/.exec(trimmed);
    if (numbered) {
      flushParagraph();
      if (list?.type !== 'numbered') {
        flushList();
        list = { type: 'numbered', items: [] };
      }
      list.items.push(numbered[1]);
      continue;
    }
    flushList();
    paragraph.push(trimmed);
  }
  flushParagraph();
  flushList();
  return blocks;
}

function inlineNodes(content: string): ReactNode[] {
  const token = /(\*\*[^*]+\*\*|\[[^\]]+\]\([^)]+\))/g;
  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const match of content.matchAll(token)) {
    const index = match.index ?? 0;
    if (index > cursor) nodes.push(content.slice(cursor, index));
    const value = match[0];
    if (value.startsWith('**')) {
      nodes.push(<Text key={`bold-${index}`} style={styles.bold}>{value.slice(2, -2)}</Text>);
    } else {
      const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(value);
      if (link) {
        nodes.push(
          <Text
            accessibilityRole="link"
            key={`link-${index}`}
            onPress={() => void Linking.openURL(link[2])}
            style={styles.link}
          >
            {link[1]}
          </Text>,
        );
      }
    }
    cursor = index + value.length;
  }
  if (cursor < content.length) nodes.push(content.slice(cursor));
  return nodes;
}

export function MarkdownContent({ value }: { value: string }) {
  const blocks = useMemo(() => parseBlocks(value), [value]);
  return (
    <View accessibilityLabel="AskU 回答正文" style={styles.root}>
      {blocks.map((block, blockIndex) => {
        if (block.type === 'heading') {
          return (
            <Text key={`heading-${blockIndex}`} style={[styles.heading, block.level > 1 && styles.headingSmall]}>
              {inlineNodes(block.content)}
            </Text>
          );
        }
        if (block.type === 'paragraph') {
          return <Text key={`paragraph-${blockIndex}`} style={styles.paragraph}>{inlineNodes(block.content)}</Text>;
        }
        return (
          <View key={`list-${blockIndex}`} style={styles.list}>
            {block.items.map((item, itemIndex) => (
              <View key={`${block.type}-${itemIndex}`} style={styles.listRow}>
                <Text style={styles.marker}>{block.type === 'numbered' ? `${itemIndex + 1}.` : '•'}</Text>
                <Text style={styles.listText}>{inlineNodes(item)}</Text>
              </View>
            ))}
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: spacing[3] },
  heading: { ...typography.heading, color: colors.textPrimary, marginTop: spacing[2] },
  headingSmall: { fontSize: 17, lineHeight: 24 },
  paragraph: { ...typography.body, color: colors.textPrimary },
  bold: { fontWeight: '700' },
  link: { color: colors.accent, textDecorationLine: 'underline' },
  list: { gap: spacing[2] },
  listRow: { flexDirection: 'row', alignItems: 'flex-start' },
  marker: { ...typography.body, width: 24, color: colors.textPrimary, fontWeight: '600' },
  listText: { ...typography.body, flex: 1, color: colors.textPrimary },
});
