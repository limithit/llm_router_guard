/**
 * 轻量自绘 SVG 图表（趋势折线图 / 环形占比图）。
 * 说明：为规避图表重型依赖（@ant-design/plots / echarts）带来的安装与构建不确定性，
 * 采用任务允许的降级方案 —— 自绘 SVG 简单趋势图，无额外运行时依赖。
 */
import { useMemo } from 'react';
import { Empty, Typography } from 'antd';

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

const COLORS = ['#1677ff', '#ff4d4f'];

export function LineChart({ data, seriesName = ['主序列', '次序列'], height = 240 }: LineChartProps) {
  const view = useMemo(() => {
    if (!data.length) return null;
    const pad = { l: 44, r: 16, t: 16, b: 28 };
    const w = 640;
    const h = height;
    const innerW = w - pad.l - pad.r;
    const innerH = h - pad.t - pad.b;
    const maxV = Math.max(1, ...data.map((d) => Math.max(d.value, d.value2 ?? 0)));
    const niceMax = Math.ceil(maxV * 1.15 / Math.pow(10, Math.floor(Math.log10(maxV)))) * Math.pow(10, Math.floor(Math.log10(maxV)));
    const x = (i: number) => pad.l + (data.length === 1 ? innerW / 2 : (i * innerW) / (data.length - 1));
    const y = (v: number) => pad.t + innerH - (v / niceMax) * innerH;
    return { pad, w, h, innerW, innerH, niceMax, x, y };
  }, [data, height]);

  if (!view) {
    return <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: 16 }} />;
  }

  const { pad, w, h, niceMax, x, y } = view;
  const gridLines = [0, 0.25, 0.5, 0.75, 1];

  const pathFor = (key: 'value' | 'value2') => {
    return data
      .map((d, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(d[key] ?? 0).toFixed(1)}`)
      .join(' ');
  };

  return (
    <div>
      <svg viewBox={`0 0 ${w} ${h}`} style={{ width: '100%', height: 'auto', display: 'block' }}>
        {gridLines.map((g) => {
          const gy = pad.t + (h - pad.t - pad.b) * (1 - g);
          return (
            <g key={g}>
              <line x1={pad.l} y1={gy} x2={w - pad.r} y2={gy} stroke="#f0f0f0" strokeDasharray="3 3" />
              <text x={pad.l - 6} y={gy + 3} textAnchor="end" fontSize={10} fill="#999">
                {Math.round(niceMax * g)}
              </text>
            </g>
          );
        })}
        {data.map((d, i) => (
          <text key={d.label} x={x(i)} y={h - 6} textAnchor="middle" fontSize={10} fill="#999">
            {d.label}
          </text>
        ))}
        <path d={pathFor('value')} fill="none" stroke={COLORS[0]} strokeWidth={2} />
        <path d={pathFor('value2')} fill="none" stroke={COLORS[1]} strokeWidth={2} strokeDasharray="4 2" />
        {data.map((d, i) => (
          <circle key={d.label} cx={x(i)} cy={y(d.value)} r={3} fill={COLORS[0]} />
        ))}
      </svg>
      <div style={{ display: 'flex', justifyContent: 'center', gap: 16, fontSize: 12, color: '#666' }}>
        <span>
          <span style={{ color: COLORS[0] }}>●</span> {seriesName[0]}
        </span>
        {data.some((d) => d.value2 !== undefined) && (
          <span>
            <span style={{ color: COLORS[1] }}>●</span> {seriesName[1]}
          </span>
        )}
      </div>
    </div>
  );
}

interface DonutChartProps {
  data: { label: string; value: number }[];
  height?: number;
  centerText?: string;
}

const DONUT_COLORS = ['#1677ff', '#f5222d', '#faad14', '#52c41a', '#722ed1', '#13c2c2', '#eb2f96'];

export function DonutChart({ data, height = 220, centerText }: DonutChartProps) {
  const total = data.reduce((s, d) => s + d.value, 0);

  if (!total || !data.length) {
    return <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} style={{ padding: 16 }} />;
  }

  const r = 60;
  const cx = 80;
  const cy = height / 2;
  const strokeW = 26;
  const c = 2 * Math.PI * r;

  let acc = 0;
  const segments = data.map((d, i) => {
    const frac = d.value / total;
    const seg = { ...d, color: DONUT_COLORS[i % DONUT_COLORS.length], frac, start: acc, end: acc + frac };
    acc += frac;
    return seg;
  });

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
      <svg width={cx * 2} height={height} style={{ display: 'block', flex: '0 0 auto' }}>
        <circle cx={cx} cy={cy} r={r} fill="none" stroke="#f0f0f0" strokeWidth={strokeW} />
        {segments.map((s) => (
          <circle
            key={s.label}
            cx={cx}
            cy={cy}
            r={r}
            fill="none"
            stroke={s.color}
            strokeWidth={strokeW}
            strokeDasharray={`${Math.max(0, (s.end - s.start) * c - 1.5)} ${c}`}
            strokeDashoffset={-s.start * c}
            transform={`rotate(-90 ${cx} ${cy})`}
          />
        ))}
        <text x={cx} y={cy - 4} textAnchor="middle" fontSize={20} fontWeight={600}>
          {total}
        </text>
        <text x={cx} y={cy + 16} textAnchor="middle" fontSize={11} fill="#999">
          {centerText ?? '合计'}
        </text>
      </svg>
      <div style={{ flex: 1, minWidth: 140 }}>
        {segments.map((s) => (
          <div key={s.label} style={{ display: 'flex', alignItems: 'center', gap: 6, margin: '3px 0' }}>
            <span style={{ width: 8, height: 8, borderRadius: 2, background: s.color, display: 'inline-block' }} />
            <Typography.Text style={{ flex: 1, fontSize: 12 }} ellipsis>
              {s.label}
            </Typography.Text>
            <Typography.Text style={{ fontSize: 12 }} type="secondary">
              {Math.round(s.frac * 1000) / 10}%
            </Typography.Text>
          </div>
        ))}
      </div>
    </div>
  );
}
