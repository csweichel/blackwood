import { useState, useRef, useCallback, useEffect } from "react";
import { EntryType, EntrySource } from "../api/types";
import { createEntryWithAttachment } from "../api/client";

interface AudioRecorderProps {
  date: string;
  onCreated: () => void;
  onClose?: () => void;
  autoStart?: boolean;
}

type RecordingState = "idle" | "recording" | "uploading";

export default function AudioRecorder({ date, onCreated, onClose, autoStart }: AudioRecorderProps) {
  const [state, setState] = useState<RecordingState>("idle");
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const animFrameRef = useRef<number | null>(null);
  const autoStartedRef = useRef(false);

  const cleanup = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    if (animFrameRef.current) {
      cancelAnimationFrame(animFrameRef.current);
      animFrameRef.current = null;
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
    analyserRef.current = null;
    mediaRecorderRef.current = null;
    chunksRef.current = [];
  }, []);

  useEffect(() => {
    return cleanup;
  }, [cleanup]);

  // Auto-start recording when mounted with autoStart prop
  useEffect(() => {
    if (autoStart && !autoStartedRef.current) {
      autoStartedRef.current = true;
      startRecording();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoStart]);

  function formatDuration(seconds: number): string {
    const m = Math.floor(seconds / 60).toString().padStart(2, "0");
    const s = (seconds % 60).toString().padStart(2, "0");
    return `${m}:${s}`;
  }

  function drawWaveform() {
    const canvas = canvasRef.current;
    const analyser = analyserRef.current;
    if (!canvas || !analyser) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const bufferLength = analyser.frequencyBinCount;
    const dataArray = new Uint8Array(bufferLength);

    function draw() {
      if (!analyserRef.current || !canvasRef.current) return;
      animFrameRef.current = requestAnimationFrame(draw);

      analyserRef.current.getByteTimeDomainData(dataArray);

      const w = canvasRef.current.width;
      const h = canvasRef.current.height;
      ctx!.clearRect(0, 0, w, h);

      ctx!.lineWidth = 2;
      ctx!.strokeStyle = getComputedStyle(canvasRef.current!).getPropertyValue("--accent").trim() || "#C4973B";
      ctx!.beginPath();

      const sliceWidth = w / bufferLength;
      let x = 0;

      for (let i = 0; i < bufferLength; i++) {
        const v = dataArray[i] / 128.0;
        const y = (v * h) / 2;
        if (i === 0) {
          ctx!.moveTo(x, y);
        } else {
          ctx!.lineTo(x, y);
        }
        x += sliceWidth;
      }

      ctx!.lineTo(w, h / 2);
      ctx!.stroke();
    }

    draw();
  }

  async function startRecording() {
    setError(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;

      // Set up audio analyser for waveform
      const audioCtx = new AudioContext();
      const source = audioCtx.createMediaStreamSource(stream);
      const analyser = audioCtx.createAnalyser();
      analyser.fftSize = 2048;
      source.connect(analyser);
      analyserRef.current = analyser;

      const mediaRecorder = new MediaRecorder(stream, {
        mimeType: MediaRecorder.isTypeSupported("audio/webm") ? "audio/webm" : "",
      });
      mediaRecorderRef.current = mediaRecorder;
      chunksRef.current = [];

      mediaRecorder.ondataavailable = (e) => {
        if (e.data.size > 0) {
          chunksRef.current.push(e.data);
        }
      };

      mediaRecorder.onstop = () => {
        handleRecordingComplete();
      };

      mediaRecorder.start();
      setState("recording");
      setDuration(0);
      durationRef.current = 0;
      timerRef.current = setInterval(() => {
        durationRef.current += 1;
        setDuration((d) => d + 1);
      }, 1000);

      // Start waveform visualization
      drawWaveform();
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("Microphone permission denied.");
      } else {
        setError("Failed to access microphone.");
      }
    }
  }

  const durationRef = useRef(0);

  function stopRecording() {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    if (animFrameRef.current) {
      cancelAnimationFrame(animFrameRef.current);
      animFrameRef.current = null;
    }

    // Discard recordings shorter than 2 seconds
    if (durationRef.current < 2) {
      cleanup();
      setState("idle");
      setDuration(0);
      onClose?.();
      return;
    }

    if (mediaRecorderRef.current && mediaRecorderRef.current.state === "recording") {
      mediaRecorderRef.current.stop();
    }
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
  }

  async function handleRecordingComplete() {
    setState("uploading");
    setError(null);
    try {
      const mimeType = mediaRecorderRef.current?.mimeType || "audio/webm";
      const ext = mimeType.includes("webm") ? "webm" : "ogg";
      const blob = new Blob(chunksRef.current, { type: mimeType });
      const file = new File([blob], `recording.${ext}`, { type: mimeType });

      await createEntryWithAttachment(
        date,
        EntryType.AUDIO,
        "",
        EntrySource.WEB,
        file
      );
      onCreated();
      onClose?.();
    } catch (err) {
      console.error("Failed to upload audio:", err);
      setError("Failed to upload audio recording.");
      setState("idle");
    } finally {
      chunksRef.current = [];
    }
  }

  if (error) {
    return (
      <div className="bg-card border border-border rounded-lg p-4 flex items-center justify-between">
        <span className="text-destructive text-sm">{error}</span>
        {onClose && (
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground text-sm ml-4">
            Dismiss
          </button>
        )}
      </div>
    );
  }

  if (state === "uploading") {
    return (
      <div className="bg-card border border-border rounded-lg p-4 flex items-center gap-3">
        <svg className="w-4 h-4 animate-spin text-accent" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <span className="text-muted-foreground text-sm">Processing audio...</span>
      </div>
    );
  }

  if (state === "recording") {
    return (
      <div className="bg-card border border-border rounded-lg p-3 flex items-center gap-3">
        {/* Recording indicator */}
        <span className="relative flex h-3 w-3 shrink-0">
          <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-destructive/60 opacity-75"></span>
          <span className="relative inline-flex rounded-full h-3 w-3 bg-destructive"></span>
        </span>

        {/* Duration */}
        <span className="text-destructive font-mono text-sm font-medium tabular-nums shrink-0">
          {formatDuration(duration)}
        </span>

        {/* Waveform */}
        <canvas
          ref={canvasRef}
          width={300}
          height={32}
          className="flex-1 h-8"
          style={{ minWidth: 0 }}
        />

        {/* Stop button — right aligned */}
        <button
          onClick={stopRecording}
          className="w-8 h-8 flex items-center justify-center rounded-full bg-primary text-primary-foreground hover:opacity-90 transition-colors shrink-0 ml-auto"
          title="Stop recording"
        >
          <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24">
            <rect x="6" y="6" width="12" height="12" rx="2" />
          </svg>
        </button>
      </div>
    );
  }

  // idle state — only shown if autoStart failed or wasn't set
  return (
    <div className="bg-card border border-border rounded-lg p-4 flex items-center justify-between">
      <button
        onClick={startRecording}
        className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground text-sm font-medium rounded-full hover:opacity-90 transition-colors"
      >
        <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
          <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
          <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
        </svg>
        Start Recording
      </button>
      {onClose && (
        <button onClick={onClose} className="text-muted-foreground hover:text-foreground text-sm">
          Cancel
        </button>
      )}
    </div>
  );
}
