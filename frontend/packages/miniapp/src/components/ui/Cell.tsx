// 通用列表行单元格(标题/描述/值/图标/箭头,可点击)
import { View, Text } from '@tarojs/components';
import './Cell.scss';

export interface CellProps {
  title: string;
  desc?: string;
  value?: string;
  icon?: string;
  arrow?: boolean;
  onClick?: () => void;
}

export function Cell({ title, desc, value, icon, arrow, onClick }: CellProps) {
  return (
    <View className={`ui-cell${onClick ? ' ui-cell--tappable' : ''}`} onClick={onClick}>
      {icon ? <Text className="ui-cell__icon">{icon}</Text> : null}
      <View className="ui-cell__main">
        <Text className="ui-cell__title">{title}</Text>
        {desc ? <Text className="ui-cell__desc">{desc}</Text> : null}
      </View>
      {value ? <Text className="ui-cell__value">{value}</Text> : null}
      {arrow ? <Text className="ui-cell__arrow">›</Text> : null}
    </View>
  );
}
