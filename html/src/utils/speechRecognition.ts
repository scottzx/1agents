// Framework-agnostic Web Speech API core.
//
// Owns the recognition lifecycle and the punctuation-aware transcript
// accumulation. Both the `useSpeechRecognition` hook (function
// components) and the class-based Terminal drive the same logic through
// this controller, so the recognition behavior lives in exactly one place.

import { t, type Lang } from '../components/i18n';

interface SpeechResultEvent {
    resultIndex: number;
    results: {
        length: number;
        [index: number]: {
            isFinal: boolean;
            [index: number]: {
                transcript: string;
            };
        };
    };
}

interface SpeechErrorEvent {
    error: string;
}

interface SpeechRecognitionInstance {
    continuous: boolean;
    interimResults: boolean;
    lang: string;
    onstart: () => void;
    onresult: (event: SpeechResultEvent) => void;
    onerror: (event: SpeechErrorEvent) => void;
    onend: () => void;
    start: () => void;
    abort: () => void;
}

type SpeechWindow = Window & {
    SpeechRecognition?: new () => SpeechRecognitionInstance;
    webkitSpeechRecognition?: new () => SpeechRecognitionInstance;
};

/** Web Speech API present in this browser? */
export function speechRecognitionSupported(): boolean {
    if (typeof window === 'undefined') return false;
    const w = window as SpeechWindow;
    return !!(w.SpeechRecognition || w.webkitSpeechRecognition);
}

/** getUserMedia (and thus speech) only works in a secure context. */
export function isSecureSpeechContext(): boolean {
    return (
        typeof window !== 'undefined' &&
        !!window.location &&
        (window.location.protocol === 'https:' ||
            window.location.hostname === 'localhost' ||
            window.location.hostname === '127.0.0.1')
    );
}

export interface SpeechControllerOptions {
    /** Resolved at each `start()` so the recognizer follows the live UI language. */
    getLanguage: () => Lang;
    /** Field value snapshotted when recording starts; the transcript appends to it. */
    getBaseValue: () => string;
    /** Called with base value + appended transcript on every result. */
    onTranscript: (next: string) => void;
    onRecordingChange: (recording: boolean) => void;
    /** Localized error message; only fired on failure. */
    onError: (message: string) => void;
}

export interface SpeechController {
    /** Start if idle, stop if recording. */
    toggle(): void;
    start(): void;
    /** Stop recognition, keep the recognized text. */
    stop(): void;
    /** Stop recognition and restore the field to its pre-recording value. */
    cancel(): void;
    /** Abort and release the mic; for unmount/teardown. */
    dispose(): void;
    readonly recording: boolean;
}

export function createSpeechController(options: SpeechControllerOptions): SpeechController {
    let recognition: SpeechRecognitionInstance | null = null;
    let startValue = '';
    let recording = false;

    const setRecording = (value: boolean) => {
        recording = value;
        options.onRecordingChange(value);
    };

    const cleanup = () => {
        if (recognition) {
            try {
                recognition.abort();
            } catch {
                // ignore abort races
            }
            recognition = null;
        }
        setRecording(false);
    };

    const start = () => {
        if (recording) return;

        const w = window as SpeechWindow;
        const SpeechRecognition = w.SpeechRecognition || w.webkitSpeechRecognition;
        if (!SpeechRecognition) {
            options.onError(t('terminal.speech.unsupported', options.getLanguage()));
            return;
        }

        // Snapshot the language for this session so the async result/error
        // handlers stay consistent even if the UI language changes mid-record.
        const language = options.getLanguage();
        const isChinese = language.toLowerCase().startsWith('zh');
        const period = t(isChinese ? 'terminal.period.zh' : 'terminal.period.en', language);

        try {
            recognition = new SpeechRecognition();
            recognition.continuous = true;
            recognition.interimResults = true;
            recognition.lang = language;

            recognition.onstart = () => {
                startValue = options.getBaseValue();
                setRecording(true);
            };

            recognition.onresult = (event: SpeechResultEvent) => {
                const finalParts: string[] = [];
                let interimText = '';

                for (let i = 0; i < event.results.length; ++i) {
                    const result = event.results[i];
                    const transcript = result[0].transcript.trim();
                    if (result.isFinal) {
                        if (transcript) {
                            if (finalParts.length > 0) {
                                const prev = finalParts[finalParts.length - 1];
                                const endsWithPunct = /[.,!?;:。，？！、：；\s]$/.test(prev);
                                if (!endsWithPunct) {
                                    finalParts[finalParts.length - 1] = prev + period;
                                }
                            }
                            finalParts.push(transcript);
                        }
                    } else {
                        interimText += transcript;
                    }
                }

                if (interimText.trim() && finalParts.length > 0) {
                    const lastFinal = finalParts[finalParts.length - 1];
                    const endsWithPunct = /[.,!?;:。，？！、：；\s]$/.test(lastFinal);
                    if (!endsWithPunct) {
                        finalParts[finalParts.length - 1] = lastFinal + period;
                    }
                }

                let currentText = finalParts.join(' ');
                if (interimText.trim()) {
                    currentText = currentText ? currentText + ' ' + interimText.trim() : interimText.trim();
                }

                const updatedValue = (startValue + (startValue ? ' ' : '') + currentText).trim();
                options.onTranscript(updatedValue);
            };

            recognition.onerror = (event: SpeechErrorEvent) => {
                console.error('Speech recognition error:', event.error);
                if (event.error === 'no-speech') {
                    cleanup();
                    return;
                }
                let errMsg = t('terminal.speech.error', language);
                if (event.error === 'not-allowed') {
                    errMsg = t('terminal.speech.micDenied', language);
                } else if (event.error === 'network') {
                    errMsg = t('terminal.speech.network', language);
                }
                options.onError(errMsg);
                cleanup();
            };

            recognition.onend = () => {
                if (recording) {
                    cleanup();
                }
            };

            recognition.start();
        } catch (err) {
            console.error('Failed to start speech recognition:', err);
            options.onError(t('terminal.speech.startFailed', language));
            cleanup();
        }
    };

    return {
        start,
        stop: cleanup,
        cancel: () => {
            cleanup();
            options.onTranscript(startValue);
        },
        toggle: () => {
            if (recording) cleanup();
            else start();
        },
        dispose: cleanup,
        get recording() {
            return recording;
        },
    };
}
