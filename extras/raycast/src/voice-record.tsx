import { Action, ActionPanel, Detail, showToast, Toast, environment } from "@raycast/api";
import { useState, useRef, useEffect } from "react";
import { execSync, spawn, ChildProcess } from "child_process";
import fs from "fs";
import path from "path";
import { submitVoiceRecording } from "./api";

export default function VoiceRecord() {
  const [isRecording, setIsRecording] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const [result, setResult] = useState<string | null>(null);
  const processRef = useRef<ChildProcess | null>(null);
  const fileRef = useRef<string>("");
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const outputPath = path.join(environment.supportPath, "recording.wav");

  useEffect(() => {
    return () => {
      // Cleanup on unmount.
      if (processRef.current) {
        processRef.current.kill("SIGTERM");
      }
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
      if (fileRef.current && fs.existsSync(fileRef.current)) {
        fs.unlinkSync(fileRef.current);
      }
    };
  }, []);

  function startRecording() {
    fileRef.current = outputPath;

    // Use macOS `rec` (from sox) or fall back to `afrecord` via script.
    // sox/rec is commonly available via Homebrew. We use the simpler
    // approach of spawning `rec` which records from the default mic.
    try {
      // Check if rec is available.
      execSync("which rec", { encoding: "utf-8" });
    } catch {
      showToast({
        style: Toast.Style.Failure,
        title: "sox not found",
        message: "Install sox via: brew install sox",
      });
      return;
    }

    const proc = spawn("rec", [outputPath, "rate", "16000", "channels", "1"], {
      stdio: "ignore",
    });
    processRef.current = proc;
    setIsRecording(true);
    setElapsed(0);

    timerRef.current = setInterval(() => {
      setElapsed((prev) => prev + 1);
    }, 1000);

    proc.on("error", (err) => {
      showToast({ style: Toast.Style.Failure, title: "Recording error", message: String(err) });
      stopRecording();
    });
  }

  async function stopRecording() {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }

    if (processRef.current) {
      processRef.current.kill("SIGTERM");
      processRef.current = null;
    }

    setIsRecording(false);

    // Wait briefly for the file to be flushed.
    await new Promise((resolve) => setTimeout(resolve, 500));

    if (!fs.existsSync(fileRef.current)) {
      await showToast({ style: Toast.Style.Failure, title: "No recording file found" });
      return;
    }

    setIsSubmitting(true);
    try {
      const data = fs.readFileSync(fileRef.current);
      const base64 = data.toString("base64");
      const entry = await submitVoiceRecording(base64, "voice-memo.wav");
      const transcription = entry.content || "(transcription pending)";
      setResult(transcription);
      await showToast({ style: Toast.Style.Success, title: "Voice memo saved" });
    } catch (error) {
      await showToast({
        style: Toast.Style.Failure,
        title: "Failed to submit recording",
        message: String(error),
      });
    } finally {
      setIsSubmitting(false);
      // Clean up the temp file.
      if (fs.existsSync(fileRef.current)) {
        fs.unlinkSync(fileRef.current);
      }
    }
  }

  function formatTime(seconds: number): string {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}:${s.toString().padStart(2, "0")}`;
  }

  if (result !== null) {
    return (
      <Detail
        markdown={`## Transcription\n\n${result}`}
        actions={
          <ActionPanel>
            <Action.CopyToClipboard title="Copy Transcription" content={result} />
            <Action title="Record Another" onAction={() => setResult(null)} />
          </ActionPanel>
        }
      />
    );
  }

  const markdown = isRecording
    ? `# Recording...\n\n⏺ **${formatTime(elapsed)}**\n\nPress **Enter** to stop and submit.`
    : isSubmitting
      ? `# Submitting...\n\nUploading and transcribing your voice memo.`
      : `# Voice Record\n\nPress **Enter** to start recording.\n\nRequires \`sox\` — install via \`brew install sox\`.`;

  return (
    <Detail
      markdown={markdown}
      isLoading={isSubmitting}
      actions={
        <ActionPanel>
          {!isRecording && !isSubmitting && (
            <Action title="Start Recording" onAction={startRecording} />
          )}
          {isRecording && <Action title="Stop & Submit" onAction={stopRecording} />}
        </ActionPanel>
      }
    />
  );
}
