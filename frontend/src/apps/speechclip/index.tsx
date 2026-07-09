/**
 * speech_clip (口播剪辑) app bundle entry. Registers its project-tab view under
 * the name declared in the backend manifest (mountPoints[].view = "SpeechClipTab").
 */
import { registerAppView } from '../../modules/appViewRegistry';
import { SpeechClipTab } from './SpeechClipTab';

registerAppView('SpeechClipTab', SpeechClipTab);
