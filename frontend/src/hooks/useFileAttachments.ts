// Preact hook for uploading files from a chat input.
//
// Mirrors useSpeechRecognition's getter/setter shape so it adapts to both
// controlled inputs (`() => state` / `setState`) and uncontrolled ones backed
// by a ref. Each uploaded file is saved to /tmp by the backend; its absolute
// path is appended to the input text (the wire format — the local agent reads
// the file from that path) and tracked as an attachment chip for display.

import { useState, useRef, useEffect, useCallback } from 'preact/hooks';
import { fsService } from '../services/fsService';

export interface FileAttachment {
    /** Absolute /tmp path returned by the backend; also lives in the input text. */
    path: string;
    /** Original client filename, for the chip label. */
    name: string;
    isImage: boolean;
    /** Object URL for an image thumbnail; undefined for non-images. */
    previewUrl?: string;
}

interface FileAttachmentsState {
    attachments: FileAttachment[];
    uploading: boolean;
    error: string;
    upload: (files: FileList | File[]) => void;
    remove: (att: FileAttachment) => void;
    /** Drop all chips (e.g. after the message is sent). Does not touch text. */
    clear: () => void;
}

/** Append a path on its own line, inserting a separator only when needed. */
function appendPath(prev: string, path: string): string {
    if (!prev) return path;
    return prev.endsWith('\n') ? prev + path : prev + '\n' + path;
}

/** Remove the line equal to `path`, leaving the rest of the text intact. */
function stripPath(text: string, path: string): string {
    const lines = text.split('\n');
    const idx = lines.indexOf(path);
    if (idx !== -1) lines.splice(idx, 1);
    return lines.join('\n');
}

export function useFileAttachments(getText: () => string, setText: (next: string) => void): FileAttachmentsState {
    const [attachments, setAttachments] = useState<FileAttachment[]>([]);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');

    // Keep latest props/state reachable from async callbacks without stale closures.
    const getTextRef = useRef(getText);
    const setTextRef = useRef(setText);
    getTextRef.current = getText;
    setTextRef.current = setText;
    const attachmentsRef = useRef<FileAttachment[]>(attachments);
    attachmentsRef.current = attachments;

    // Revoke any outstanding object URLs on unmount.
    useEffect(
        () => () => {
            for (const a of attachmentsRef.current) {
                if (a.previewUrl) URL.revokeObjectURL(a.previewUrl);
            }
        },
        []
    );

    const upload = useCallback((files: FileList | File[]) => {
        const list = Array.from(files);
        if (!list.length) return;

        setUploading(true);
        void (async () => {
            // Accumulate locally so sequential appends don't read a stale text
            // snapshot (the controlled setter batches across awaits).
            let text = getTextRef.current();
            try {
                for (const file of list) {
                    const { path, name } = await fsService.upload(file);
                    text = appendPath(text, path);
                    setTextRef.current(text);
                    const isImage = file.type.startsWith('image/');
                    setAttachments(prev => [
                        ...prev,
                        { path, name, isImage, previewUrl: isImage ? URL.createObjectURL(file) : undefined },
                    ]);
                }
            } catch (e) {
                setError(e instanceof Error ? e.message : String(e));
                setTimeout(() => setError(''), 4000);
            } finally {
                setUploading(false);
            }
        })();
    }, []);

    const remove = useCallback((att: FileAttachment) => {
        setTextRef.current(stripPath(getTextRef.current(), att.path));
        if (att.previewUrl) URL.revokeObjectURL(att.previewUrl);
        setAttachments(prev => prev.filter(a => a.path !== att.path));
    }, []);

    const clear = useCallback(() => {
        for (const a of attachmentsRef.current) {
            if (a.previewUrl) URL.revokeObjectURL(a.previewUrl);
        }
        setAttachments([]);
    }, []);

    return { attachments, uploading, error, upload, remove, clear };
}
