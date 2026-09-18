/**
 * ECharts 图表封装：趋势折线图 / 环形占比图。
 * 组件 props 与旧自绘 SVG 版完全兼容（LineChart / DonutChart），页面零改动。
 * 按需注册（echarts/core）：仅折线 + 饼图 + 网格/提示/图例/标题，控制打包体积；
 * echarts 仅被懒加载页面（Dashboard / TokenStats）引用，随路由 chunk 按需加载。
 */
import { useMemo } from 'react';
import { Empty } from 'antd';
import { useTranslation } from 'react-i18next';
import * as echarts from 'echarts/core';
import { LineChart as ELine, PieChart as EPie } from 'echarts/charts';
import {
  GridComponent,
  LegendComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import ReactECharts from 'echarts-for-react/lib/core';
import type { EChartsCoreOption } from 'echarts/core';

echarts.use([ELine, EPie, GridComponent, LegendComponent, TitleComponent, TooltipComponent, CanvasRenderer]);

const COLORS = ['#1677ff', '#ff4d4f'];
const DONUT_COLORS = ['#1677ff', '#f5222d', '#faad14', '#52c41a', '#722ed1', '#13c2c2', '#eb2f96'];

function NoData() {
  const { t } = useTranslation();
  return <Empty description={t('common.noData')} image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: 16 }} />;
}

// ---------- 折线趋势 ----------

interface Point {
  label: string;
  value: number;
  value2?: number;
}

interface LineChartProps {
  data: Point[];
  /** 两条序列的名称：主序列、次序列 */
  seriesName?: [string, string];
  height?: number;
}

export function LineChart({ data, seriesName, height = 240 }: LineChartProps) {
  const { t } = useTranslation();
  const names = seriesName ?? [t('charts.primarySeries'), t('charts.secondarySeries')];
  const hasSecond = data.some((d) => d.value2 !== undefined);

  const option = useMemo<EChartsCoreOption>(() => {
    const series: Record<string, unknown>[] = [
      {
        name: names[0],
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        data: data.map((d) => d.value),
        itemStyle: { color: COLORS[0] },
        lineStyle: { color: COLORS[0], width: 2 },
      },
    ];
    if (hasSecond) {
      series.push({
        name: names[1],
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: data.map((d) => d.value2 ?? 0),
        itemStyle: { color: COLORS[1] },
        lineStyle: { color: COLORS[1], width: 2, type: 'dashed' },
      });
    }
    return {
      animation: false,
      grid: { left: 48, right: 16, top: 28, bottom: 28 },
      tooltip: { trigger: 'axis' },
      legend: hasSecond
        ? { data: names, top: 0, textStyle: { fontSize: 12 } }
        : { show: false },
      xAxis: {
        type: 'category',
        data: data.map((d) => d.label),
        axisLabel: { fontSize: 10, color: '#999' },
        axisLine: { lineStyle: { color: '#e0e0e0' } },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        axisLabel: { fontSize: 10, color: '#999' },
        splitLine: { lineStyle: { color: '#f0f0f0', type: 'dashed' } },
      },
      series,
    };
  }, [data, names, hasSecond]);

  if (!data.length) return <NoData />;
  return <ReactECharts echarts={echarts} option={option} notMerge lazyUpdate style={{ height, width: '100%' }} />;
}

// ---------- 环形占比 ----------

interface DonutChartProps {
  data: { label: string; value: number }[];
  height?: number;
  centerText?: string;
}

export function DonutChart({ data, height = 220, centerText }: DonutChartProps) {
  const { t } = useTranslation();
  const total = data.reduce((s, d) => s + d.value, 0);

  const option = useMemo<EChartsCoreOption>(() => {
    const items = [...data].sort((a, b) => b.value - a.value);
    return {
      animation: false,
      tooltip: { trigger: 'item', formatter: '{b}: {c}（{d}%）' },
      legend: {
        type: 'scroll',
        orient: 'vertical',
        right: 0,
        top: 'middle',
        itemWidth: 10,
        itemHeight: 10,
        textStyle: { fontSize: 12 },
        formatter: (name: string) => {
          const item = data.find((d) => d.label === name);
          if (!item || !total) return name;
          const pct = Math.round((item.value / total) * 1000) / 10;
          return `${name}  ${pct}%`;
        },
      },
      title: {
        text: String(total),
        subtext: centerText ?? t('charts.total'),
        left: '31%',
        top: '38%',
        textAlign: 'center',
        textStyle: { fontSize: 20, fontWeight: 600 },
        subtextStyle: { fontSize: 11, color: '#999' },
      },
      series: [
        {
          name: centerText ?? t('charts.total'),
          type: 'pie',
          radius: ['55%', '78%'],
          center: ['32%', '50%'],
          avoidLabelOverlap: false,
          label: { show: false },
          labelLine: { show: false },
          data: items.map((d, i) => ({
            name: d.label,
            value: d.value,
            itemStyle: { color: DONUT_COLORS[i % DONUT_COLORS.length] },
          })),
        },
      ],
    };
  }, [data, total, centerText, t]);

  if (!total || !data.length) return <NoData />;
  return <ReactECharts echarts={echarts} option={option} notMerge lazyUpdate style={{ height, width: '100%' }} />;
}
