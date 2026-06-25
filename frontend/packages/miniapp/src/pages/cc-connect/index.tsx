// cc-connect sub-page — embeds the web's cc-connect module (channels) in a
// <web-view>. Unlike skills, cc-connect is workspace-scoped and needs its own
// ManagementToken: we bootstrap the first workspace, ask the backend to mint the
// tokened login URL (workspaceService.getCcConnectUrl — the ManagementToken is
// baked in there), then absolutize it onto BACKEND_BASE and carry the access
// token for the gate. The client only ever handles the access token.
import { WebView } from '@tarojs/components';
import { useEffect, useState } from 'react';

import { workspaceService } from '@1agents/core/services/workspaceService';

import { Screen } from '../../components/Screen';
import { Loading } from '../../components/ui/Loading';
import { EmptyState } from '../../components/ui/EmptyState';
import { useT, useUI } from '../../hooks/useUI';
import { absoluteBackendUrl } from '../../embed';

export default function CcConnect() {
  const t = useT();
  const { theme, lang } = useUI();
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const wss = await workspaceService.list();
        const ws = wss[0];
        if (!ws) {
          if (!cancelled) setError(t('ccConnect.noWorkspace'));
          return;
        }
        const rel = await workspaceService.getCcConnectUrl(ws.id, theme, lang);
        if (!cancelled) setUrl(absoluteBackendUrl(rel));
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (error) {
    return (
      <Screen titleKey="more.ccConnect">
        <EmptyState icon="⚠️" title={t('ccConnect.error')} desc={error} />
      </Screen>
    );
  }
  if (!url) {
    return (
      <Screen titleKey="more.ccConnect">
        <Loading text={t('ccConnect.loading')} />
      </Screen>
    );
  }
  return <WebView src={url} />;
}
