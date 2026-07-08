/**
 * Studio recorder helper — manages dual video and audio recording from the browser.
 */

export interface RecordedAssets {
    webcamBlob: Blob;
    screenBlob: Blob;
    audioBlob: Blob;
    pcmData: Int16Array;
}

export class StudioRecorder {
    private webcamStream: MediaStream | null = null;
    private screenStream: MediaStream | null = null;
    private webcamOnlyStream: MediaStream | null = null;
    private screenOnlyStream: MediaStream | null = null;
    private audioOnlyStream: MediaStream | null = null;

    private webcamRecorder: MediaRecorder | null = null;
    private screenRecorder: MediaRecorder | null = null;
    private audioRecorder: MediaRecorder | null = null;

    private webcamChunks: Blob[] = [];
    private screenChunks: Blob[] = [];
    private audioChunks: Blob[] = [];

    private isRecording = false;

    /** Start recording webcam (video + audio) and screen (video only). */
    async start(audioDeviceId?: string): Promise<{ webcamStream: MediaStream; screenStream: MediaStream }> {
        if (this.isRecording) {
            throw new Error('Already recording');
        }

        this.webcamChunks = [];
        this.screenChunks = [];
        this.audioChunks = [];

        // 1. Capture webcam (video) + mic (audio)
        this.webcamStream = await navigator.mediaDevices.getUserMedia({
            video: { width: 1280, height: 720 },
            audio: audioDeviceId
                ? {
                      deviceId: { exact: audioDeviceId },
                      echoCancellation: true,
                      noiseSuppression: true,
                      autoGainControl: true,
                  }
                : {
                      echoCancellation: true,
                      noiseSuppression: true,
                      autoGainControl: true,
                  },
        });

        // 2. Capture screen (video only)
        // Request the user to select the specific window or screen displaying the HTML webpage
        this.screenStream = await navigator.mediaDevices.getDisplayMedia({
            video: { displaySurface: 'window' },
            audio: false, // We do not need system audio
        });

        // 3. Configure recorders
        // We record the webcam stream (video track only) and screen stream (video track only)
        // separately. The audio track from getUserMedia is recorded separately to generate
        // the master audio file and PCM transcription.
        const webcamVideoTrack = this.webcamStream.getVideoTracks()[0];
        const micAudioTrack = this.webcamStream.getAudioTracks()[0];
        const micAudioTrackClone = micAudioTrack.clone(); // Clone to prevent resource sharing lock

        // Create streams for each recorder
        this.webcamOnlyStream = new MediaStream([webcamVideoTrack, micAudioTrack]);
        this.screenOnlyStream = new MediaStream([this.screenStream.getVideoTracks()[0]]);
        this.audioOnlyStream = new MediaStream([micAudioTrackClone]);

        // Select supported mime types
        const videoMime = MediaRecorder.isTypeSupported('video/webm;codecs=vp9')
            ? 'video/webm;codecs=vp9'
            : MediaRecorder.isTypeSupported('video/webm;codecs=vp8')
              ? 'video/webm;codecs=vp8'
              : 'video/webm';

        const audioMime = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
            ? 'audio/webm;codecs=opus'
            : 'audio/webm';

        this.webcamRecorder = new MediaRecorder(this.webcamOnlyStream, { mimeType: videoMime });
        this.screenRecorder = new MediaRecorder(this.screenOnlyStream, { mimeType: videoMime });
        this.audioRecorder = new MediaRecorder(this.audioOnlyStream, { mimeType: audioMime });

        // Bind data handlers
        this.webcamRecorder.ondataavailable = e => {
            if (e.data && e.data.size > 0) this.webcamChunks.push(e.data);
        };
        this.screenRecorder.ondataavailable = e => {
            if (e.data && e.data.size > 0) this.screenChunks.push(e.data);
        };
        this.audioRecorder.ondataavailable = e => {
            if (e.data && e.data.size > 0) this.audioChunks.push(e.data);
        };

        // Start recorders in sync
        this.webcamRecorder.start(1000); // 1s slices
        this.screenRecorder.start(1000);
        this.audioRecorder.start(1000);

        this.isRecording = true;

        return {
            webcamStream: this.webcamStream,
            screenStream: this.screenStream,
        };
    }

    /** Stop all recorders and process files. */
    stop(): Promise<RecordedAssets> {
        return new Promise((resolve, reject) => {
            if (!this.isRecording) {
                reject(new Error('Not recording'));
                return;
            }

            let stopCount = 0;
            const checkResolve = async () => {
                stopCount++;
                if (stopCount < 3) return; // Wait for all 3 recorders to stop

                // Stop track captures after all recorders finished flushing
                if (this.webcamStream) {
                    this.webcamStream.getTracks().forEach(t => t.stop());
                }
                if (this.audioOnlyStream) {
                    this.audioOnlyStream.getTracks().forEach(t => t.stop());
                }
                if (this.screenStream) {
                    this.screenStream.getTracks().forEach(t => t.stop());
                }

                try {
                    const webcamBlob = new Blob(this.webcamChunks, { type: 'video/webm' });
                    const screenBlob = new Blob(this.screenChunks, { type: 'video/webm' });
                    const audioBlob = new Blob(this.audioChunks, { type: 'audio/webm' });

                    // Decode audio blob to PCM 16kHz mono
                    const pcmData = await this.decodeAudioToPcm16(audioBlob);

                    this.isRecording = false;
                    this.webcamStream = null;
                    this.screenStream = null;
                    this.webcamOnlyStream = null;
                    this.screenOnlyStream = null;
                    this.audioOnlyStream = null;

                    resolve({
                        webcamBlob,
                        screenBlob,
                        audioBlob,
                        pcmData,
                    });
                } catch (err) {
                    reject(err);
                }
            };

            if (this.webcamRecorder) {
                this.webcamRecorder.onstop = checkResolve;
                this.webcamRecorder.stop();
            }
            if (this.screenRecorder) {
                this.screenRecorder.onstop = checkResolve;
                this.screenRecorder.stop();
            }
            if (this.audioRecorder) {
                this.audioRecorder.onstop = checkResolve;
                this.audioRecorder.stop();
            }
        });
    }

    /** Decode standard browser audio blob (WebM/Opus) and downsample to 16kHz mono PCM (Int16Array). */
    private async decodeAudioToPcm16(blob: Blob): Promise<Int16Array> {
        const arrayBuffer = await blob.arrayBuffer();
        const AudioCtx =
            window.AudioContext ||
            (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
        const ctx = new AudioCtx();

        // Decode compressed audio data
        const audioBuffer = await ctx.decodeAudioData(arrayBuffer);
        await ctx.close();

        const channelData = audioBuffer.getChannelData(0); // Left channel
        const sampleRate = audioBuffer.sampleRate;
        const targetSampleRate = 16000;

        if (sampleRate === targetSampleRate) {
            // Already 16kHz
            const pcm = new Int16Array(channelData.length);
            for (let i = 0; i < channelData.length; i++) {
                const s = Math.max(-1, Math.min(1, channelData[i]));
                pcm[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
            }
            return pcm;
        }

        // Downsample
        const ratio = sampleRate / targetSampleRate;
        const newLength = Math.round(channelData.length / ratio);
        const pcm = new Int16Array(newLength);

        for (let i = 0; i < newLength; i++) {
            const offset = Math.round(i * ratio);
            if (offset >= channelData.length) break;
            const s = Math.max(-1, Math.min(1, channelData[offset]));
            pcm[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
        }

        return pcm;
    }
}
