// Preact hook wrapping the shared speech-recognition controller.
//
// Adapts the framework-agnostic core (src/utils/speechRecognition.ts)
// to a controlled text field in a function component. The Terminal
// (a class component) drives the same controller directly.

import { useRef, useState, useEffect, useCallback } from 'preact/hooks';
import type { Lang } from '../components/i18n';
import {
    createSpeechController,
    speechRecognitionSupported,
    isSecureSpeechContext,
    type SpeechController,
} from '../utils/speechRecognition';

interface SpeechRecognitionState {
    /** True when speech recognition is usable here (API + secure context). */
    available: boolean;
    isRecording: boolean;
    error: string;
    toggle: () => void;
}

/**
 * Drive system speech recognition for a text field.
 *
 * Takes getter/setter rather than a value snapshot so it works for both
 * controlled inputs (`() => state`) and uncontrolled ones backed by a
 * ref (`() => ref.current?.value ?? ''`).
 *
 * @param language UI language → recognition locale + punctuation.
 * @param getText  Reads the current field text when recording starts.
 * @param setText  Called with base text + appended transcript on each result.
 */
export function useSpeechRecognition(
    language: Lang,
    getText: () => string,
    setText: (next: string) => void
): SpeechRecognitionState {
    const [isRecording, setIsRecording] = useState(false);
    const [error, setError] = useState('');

    // Keep the latest props reachable from the long-lived controller
    // without re-creating it on every render.
    const languageRef = useRef(language);
    const getTextRef = useRef(getText);
    const setTextRef = useRef(setText);
    languageRef.current = language;
    getTextRef.current = getText;
    setTextRef.current = setText;

    const controllerRef = useRef<SpeechController | null>(null);
    if (!controllerRef.current) {
        controllerRef.current = createSpeechController({
            getLanguage: () => languageRef.current,
            getBaseValue: () => getTextRef.current(),
            onTranscript: next => setTextRef.current(next),
            onRecordingChange: setIsRecording,
            onError: msg => {
                setError(msg);
                setTimeout(() => setError(''), 4000);
            },
        });
    }

    // Release the mic on unmount.
    useEffect(() => () => controllerRef.current?.dispose(), []);

    const toggle = useCallback(() => controllerRef.current?.toggle(), []);

    return {
        available: speechRecognitionSupported() && isSecureSpeechContext(),
        isRecording,
        error,
        toggle,
    };
}
