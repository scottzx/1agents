import { h } from 'preact';
import { useRef, useEffect } from 'preact/hooks';
import * as echarts from 'echarts/core';
import { PieChart, BarChart } from 'echarts/charts';
import { TooltipComponent, GridComponent, LegendComponent, TitleComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsOption } from 'echarts';

import { theme } from '../../../stores/uiStore';

// Register only the chart types / components the dashboard widgets use, so
// webpack tree-shakes the rest of ECharts out of the bundle.
echarts.use([PieChart, BarChart, TooltipComponent, GridComponent, LegendComponent, TitleComponent, CanvasRenderer]);

interface EChartProps {
    option: EChartsOption;
    height?: number | string;
    class?: string;
}

/**
 * Thin Preact wrapper around an ECharts instance. Inits once on mount,
 * re-applies the option whenever it (or the theme) changes, auto-resizes via
 * ResizeObserver, and disposes on unmount. Reading `theme.value` in render
 * subscribes the wrapper so palette swaps re-render the chart; callers pass
 * options whose colors are resolved from CSS tokens for the active theme.
 */
export function EChart({ option, height = 240, class: className }: EChartProps) {
    const el = useRef<HTMLDivElement>(null);
    const chart = useRef<echarts.ECharts | null>(null);

    // Subscribe to theme changes so the effect below re-runs on palette swap.
    const activeTheme = theme.value;

    useEffect(() => {
        if (!el.current) return;
        const instance = echarts.init(el.current);
        chart.current = instance;
        const ro = new ResizeObserver(() => instance.resize());
        ro.observe(el.current);
        return () => {
            ro.disconnect();
            instance.dispose();
            chart.current = null;
        };
    }, []);

    useEffect(() => {
        // `true` = notMerge: fully replace so removed series/legend entries drop.
        chart.current?.setOption(option, true);
    }, [option, activeTheme]);

    return (
        <div
            ref={el}
            class={className}
            style={{ width: '100%', height: typeof height === 'number' ? `${height}px` : height }}
        />
    );
}
