// 技能中心 tab — embeds the web's 1skills module in a <web-view> (cross-host
// equivalent of the desktop/web iframe embed). The page is served by the backend
// at /1skills/; the access token in the URL gets it past the gate, and 1skills'
// own /api/skills calls ride the cookie the gate seeds. No re-implementation in Taro.
import { WebView } from '@tarojs/components';
import { useUI } from '../../hooks/useUI';
import { skillsEmbedUrl } from '../../embed';

export default function Skills() {
  const { theme, lang } = useUI();
  // Built once at mount; a theme/lang change applies on the next visit (web-view
  // can't be live-themed without a reload — acceptable for an embedded surface).
  return <WebView src={skillsEmbedUrl(theme, lang)} />;
}
