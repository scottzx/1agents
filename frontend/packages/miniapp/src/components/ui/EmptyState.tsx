// Empty / error placeholder — centered icon + title + optional desc & action.
import { View, Text, Button } from '@tarojs/components';
import './EmptyState.scss';

export interface EmptyStateAction {
  label: string;
  onClick: () => void;
}

export interface EmptyStateProps {
  icon?: string;
  title: string;
  desc?: string;
  action?: EmptyStateAction;
}

export function EmptyState({ icon, title, desc, action }: EmptyStateProps) {
  return (
    <View className="ui-empty">
      <Text className="ui-empty__icon">{icon || '📭'}</Text>
      <Text className="ui-empty__title">{title}</Text>
      {desc ? <Text className="ui-empty__desc">{desc}</Text> : null}
      {action ? (
        <Button className="ui-empty__action" onClick={action.onClick}>
          {action.label}
        </Button>
      ) : null}
    </View>
  );
}
