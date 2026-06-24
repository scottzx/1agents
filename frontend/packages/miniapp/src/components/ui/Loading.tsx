// Centered spinner with optional caption.
import { View, Text } from '@tarojs/components';
import './Loading.scss';

export interface LoadingProps {
  text?: string;
}

export function Loading({ text }: LoadingProps) {
  return (
    <View className="ui-loading">
      <View className="ui-loading__spinner" />
      {text ? <Text className="ui-loading__text">{text}</Text> : null}
    </View>
  );
}
