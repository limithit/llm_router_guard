import { Button, Result } from 'antd';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

export default function NotFound() {
  const { t } = useTranslation();
  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#f0f2f5',
      }}
    >
      <Result
        status="404"
        title="404"
        subTitle={t('notFound.subTitle')}
        extra={
          <Link to="/">
            <Button type="primary">{t('notFound.backHome')}</Button>
          </Link>
        }
      />
    </div>
  );
}
