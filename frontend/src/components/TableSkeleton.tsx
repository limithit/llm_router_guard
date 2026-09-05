/**
 * TableSkeleton — 表格加载态骨架屏（Minor #9）。
 * antd Table 的 loading prop 支持 SpinProps.indicator 自定义指示器：
 * 首查（尚无数据）用骨架行替代转圈遮罩，翻页/操作保持轻量转圈；
 * 消除"已有数据区域被清空后整块转圈"的体验断裂。
 */
import { Skeleton } from 'antd';
import type { TableProps } from 'antd';

export default function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div style={{ minWidth: 420, padding: '4px 8px', textAlign: 'left' }}>
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton
          key={i}
          active
          title={false}
          paragraph={{ rows: 1, width: `${72 + ((i * 13) % 22)}%` }}
          style={{ marginBottom: 18 }}
        />
      ))}
    </div>
  );
}

/**
 * 生成 Table 的 loading prop：
 * @param initial 首次查询加载中（useQuery isLoading）
 * @param busy    操作/删除/重置等忙碌态（保留默认转圈）
 * @param hasData 当前是否已有行数据（有数据时首查刷新也用转圈，避免重建骨架）
 */
export function tableLoading(
  initial: boolean,
  busy: boolean | undefined,
  hasData: boolean,
): TableProps['loading'] {
  if (initial && !hasData) return { indicator: <TableSkeleton /> };
  return Boolean(initial || busy);
}
