// Card surface — bento-style container. Pass onClick to make it pressable.
import type { ReactNode } from 'react';
import { View } from '@tarojs/components';
import './Card.scss';

export interface CardProps {
  className?: string;
  onClick?: () => void;
  children: ReactNode;
}

export function Card({ className, onClick, children }: CardProps) {
  const cls = `ui-card${onClick ? ' ui-card--interactive' : ''}${className ? ` ${className}` : ''}`;
  return (
    <View className={cls} onClick={onClick}>
      {children}
    </View>
  );
}
