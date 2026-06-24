// Titled group — small uppercase muted header above a body of rows/cards.
import type { ReactNode } from 'react';
import { View, Text } from '@tarojs/components';
import './Section.scss';

export interface SectionProps {
  title?: string;
  children: ReactNode;
  className?: string;
}

export function Section({ title, children, className }: SectionProps) {
  return (
    <View className={`ui-section${className ? ` ${className}` : ''}`}>
      {title ? <Text className="ui-section__title">{title}</Text> : null}
      <View className="ui-section__body">{children}</View>
    </View>
  );
}
