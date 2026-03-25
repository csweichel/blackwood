import { getPreferenceValues } from "@raycast/api";
import fetch from "node-fetch";
import fs from "fs";
import path from "path";

interface Preferences {
  serverUrl: string;
}

function baseUrl(): string {
  const { serverUrl } = getPreferenceValues<Preferences>();
  return serverUrl.replace(/\/+$/, "");
}

// --- Connect-RPC JSON helpers ---

/** Call a Connect-RPC unary method using the Connect protocol (JSON). */
async function connectUnary<Req, Res>(service: string, method: string, request: Req): Promise<Res> {
  const url = `${baseUrl()}/${service}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Connect-Protocol-Version": "1",
    },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`${method} failed (${resp.status}): ${text}`);
  }
  return (await resp.json()) as Res;
}

/**
 * Call a Connect-RPC server-streaming method and collect all response messages.
 *
 * Connect server-streaming over JSON uses a binary envelope format:
 * each message is prefixed with a 1-byte flags field and a 4-byte big-endian
 * length, followed by the JSON-encoded message body.
 *
 * The last envelope (flags & 0x02) is the trailers-only frame.
 */
async function connectServerStreamCollect<Req, Res>(
  service: string,
  method: string,
  request: Req,
): Promise<Res[]> {
  const url = `${baseUrl()}/${service}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Connect-Protocol-Version": "1",
    },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`${method} stream failed (${resp.status}): ${text}`);
  }

  const buf = Buffer.from(await resp.arrayBuffer());
  const results: Res[] = [];
  let offset = 0;

  while (offset < buf.length) {
    if (offset + 5 > buf.length) break;
    const flags = buf[offset];
    const len = buf.readUInt32BE(offset + 1);
    offset += 5;
    if (offset + len > buf.length) break;

    const payload = buf.subarray(offset, offset + len);
    offset += len;

    // flags & 0x02 indicates the trailers frame (end-of-stream).
    if (flags & 0x02) {
      // Trailers frame — check for errors.
      const trailers = JSON.parse(payload.toString("utf-8"));
      if (trailers.error) {
        throw new Error(trailers.error.message || "stream error");
      }
      break;
    }

    results.push(JSON.parse(payload.toString("utf-8")) as Res);
  }

  return results;
}

// --- REST helpers ---

async function restPost<Req, Res>(path: string, body: Req): Promise<Res> {
  const url = `${baseUrl()}${path}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`POST ${path} failed (${resp.status}): ${text}`);
  }
  return (await resp.json()) as Res;
}

// --- Entry types matching proto enums ---

export enum EntryType {
  TEXT = "ENTRY_TYPE_TEXT",
  AUDIO = "ENTRY_TYPE_AUDIO",
  PHOTO = "ENTRY_TYPE_PHOTO",
}

export enum EntrySource {
  API = "ENTRY_SOURCE_API",
}

// --- DailyNotesService ---

export interface CreateEntryRequest {
  date?: string;
  type: string;
  content: string;
  source: string;
  metadata?: string;
  attachmentData?: string[]; // base64-encoded
  attachmentFilenames?: string[];
  attachmentContentTypes?: string[];
}

export interface Entry {
  id: string;
  dailyNoteId: string;
  type: string;
  content: string;
  createdAt?: string;
}

export async function createEntry(req: CreateEntryRequest): Promise<Entry> {
  return connectUnary<CreateEntryRequest, Entry>(
    "blackwood.v1.DailyNotesService",
    "CreateEntry",
    req,
  );
}

// --- ChatService ---

export interface ChatResponse {
  conversationId: string;
  content: string;
  done: boolean;
  sources?: SourceReference[];
}

export interface SourceReference {
  entryId: string;
  dailyNoteDate: string;
  snippet: string;
  score: number;
}

export async function chatStream(
  message: string,
  conversationId?: string,
): Promise<{ fullResponse: string; conversationId: string; sources: SourceReference[] }> {
  const chunks = await connectServerStreamCollect<
    { message: string; conversationId?: string },
    ChatResponse
  >("blackwood.v1.ChatService", "Chat", {
    message,
    conversationId: conversationId || "",
  });

  let fullResponse = "";
  let convId = conversationId || "";
  let sources: SourceReference[] = [];

  for (const chunk of chunks) {
    if (chunk.conversationId) convId = chunk.conversationId;
    if (chunk.content) fullResponse += chunk.content;
    if (chunk.done && chunk.sources) {
      sources = chunk.sources;
    }
  }

  return { fullResponse, conversationId: convId, sources };
}

// --- Clip endpoint (REST) ---

export interface ClipResponse {
  date: string;
}

export async function clipUrl(url: string): Promise<ClipResponse> {
  return restPost<{ url: string }, ClipResponse>("/api/clip", { url });
}

// --- Image ingestion helper ---

export async function ingestImage(filePath: string): Promise<Entry> {
  const data = fs.readFileSync(filePath);
  const base64 = data.toString("base64");
  const ext = path.extname(filePath).toLowerCase();
  const contentTypeMap: Record<string, string> = {
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".png": "image/png",
    ".gif": "image/gif",
    ".webp": "image/webp",
    ".heic": "image/heic",
  };
  const contentType = contentTypeMap[ext] || "image/jpeg";
  const filename = path.basename(filePath);

  return createEntry({
    type: EntryType.PHOTO,
    source: EntrySource.API,
    content: "",
    attachmentData: [base64],
    attachmentFilenames: [filename],
    attachmentContentTypes: [contentType],
  });
}

// --- Voice recording helper ---

export async function submitVoiceRecording(audioBase64: string, filename: string): Promise<Entry> {
  const ext = filename.split(".").pop() || "wav";
  const contentType = ext === "wav" ? "audio/wav" : `audio/${ext}`;
  return createEntry({
    type: EntryType.AUDIO,
    source: EntrySource.API,
    content: "",
    attachmentData: [audioBase64],
    attachmentFilenames: [filename],
    attachmentContentTypes: [contentType],
  });
}

// --- Health check ---

export interface HealthCheckResponse {
  status: string;
  version: string;
}

export async function healthCheck(): Promise<HealthCheckResponse> {
  return connectUnary<Record<string, never>, HealthCheckResponse>(
    "blackwood.v1.HealthService",
    "Check",
    {},
  );
}
