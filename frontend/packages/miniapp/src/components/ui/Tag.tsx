// Small status pill — color keyed by semantic tone.
import { Text } from '@tarojs/components';
import './Tag.scss';

export type TagTone = 'accent' | 'success' | 'danger' | 'warning' | 'muted';

export interface TagProps {
  text: string;
  tone?: TagTone;
}

export function Tag({ text, tone = 'muted' }: TagProps) {
  return <Text className={`ui-tag ui-tag--${tone}`}>{text}</Text>;
}
