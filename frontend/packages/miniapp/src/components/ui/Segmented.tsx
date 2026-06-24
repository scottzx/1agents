// 分段切换控件(泛型选项,受控)
import { View } from '@tarojs/components';
import './Segmented.scss';

export interface SegmentedOption<T> {
  value: T;
  label: string;
}

export interface SegmentedProps<T> {
  options: SegmentedOption<T>[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
}

export function Segmented<T extends string>({ options, value, onChange, className }: SegmentedProps<T>) {
  return (
    <View className={`seg${className ? ` ${className}` : ''}`}>
      {options.map(o => (
        <View
          key={o.value}
          className={`seg__item${o.value === value ? ' seg__item--active' : ''}`}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </View>
      ))}
    </View>
  );
}
