import { useState } from 'react';
import { Switch, message } from 'antd';

interface StatusSwitchProps {
  checked: boolean;
  /** 返回 Promise 的切换动作（如调用更新接口），失败时自动回滚并提示 */
  onChange: (checked: boolean) => Promise<void>;
  disabled?: boolean;
  size?: 'default' | 'small';
  checkedChildren?: React.ReactNode;
  unCheckedChildren?: React.ReactNode;
}

/**
 * 启用/禁用 Switch：提交期间 loading，失败自动回滚，避免表格状态与实际不一致。
 */
export default function StatusSwitch({
  checked,
  onChange,
  disabled,
  size,
  checkedChildren,
  unCheckedChildren,
}: StatusSwitchProps) {
  const [loading, setLoading] = useState(false);

  const handleChange = async (next: boolean) => {
    if (loading) return;
    setLoading(true);
    try {
      await onChange(next);
    } catch (e) {
      message.error(e instanceof Error ? e.message : '操作失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Switch
      checked={checked}
      onChange={handleChange}
      loading={loading}
      disabled={disabled}
      size={size}
      checkedChildren={checkedChildren}
      unCheckedChildren={unCheckedChildren}
    />
  );
}
