// 扩展中心 tab — embeds HarnessKit in a <web-view> for cross-host clients.
// The page is served by 1agents at /extensions/ and all data requests stay
// behind the authenticated /api/harnesskit boundary.
import { WebView } from '@tarojs/components';
import { useUI } from '../../hooks/useUI';
import { skillsEmbedUrl } from '../../embed';

export default function Skills() {
  const { theme, lang } = useUI();
  // Built once at mount; a theme/lang change applies on the next visit (web-view
  // can't be live-themed without a reload — acceptable for an embedded surface).
  return <WebView src={skillsEmbedUrl(theme, lang)} />;
}
