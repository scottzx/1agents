/**
 * speech_clip app bundle entry. Registers the project-tab view under the
 * current content-studio name, while keeping the old view name as a compatibility alias.
 */
import { registerAppView } from '../../modules/appViewRegistry';
import { SpeechClipTab } from './SpeechClipTab';

registerAppView('ContentStudioTab', SpeechClipTab);
registerAppView('SpeechClipTab', SpeechClipTab);
