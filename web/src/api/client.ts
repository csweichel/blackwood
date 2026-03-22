import type {
  DailyNote,
  Entry,
  ListDailyNotesResponse,
  CreateEntryRequest,
  DeleteEntryRequest,
  GetDailyNoteRequest,
  ListDailyNotesRequest,
  ListConversationsResponse,
  Conversation,
  SourceReference,
} from "./types";
import { type EntryType, type EntrySource } from "./types";

const DAILY_NOTES_SERVICE = "/blackwood.v1.DailyNotesService";
const CHAT_SERVICE = "/blackwood.v1.ChatService";

// Connect-go uses POST with JSON body and Content-Type: application/json.
// Field names use camelCase in the JSON wire format (protobuf JSON mapping).
async function rpc<Req, Res>(method: string, request: Req, service: string = DAILY_NOTES_SERVICE): Promise<Res> {
  const url = `${service}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`RPC ${method} failed (${resp.status}): ${text}`);
  }
  return resp.json();
}

export async function getDailyNote(req: GetDailyNoteRequest): Promise<DailyNote> {
  return rpc<GetDailyNoteRequest, DailyNote>("GetDailyNote", req);
}

export async function listDailyNotes(req: ListDailyNotesRequest): Promise<ListDailyNotesResponse> {
  return rpc<ListDailyNotesRequest, ListDailyNotesResponse>("ListDailyNotes", req);
}

export async function createEntry(req: CreateEntryRequest): Promise<Entry> {
  return rpc<CreateEntryRequest, Entry>("CreateEntry", req);
}

export async function deleteEntry(req: DeleteEntryRequest): Promise<void> {
  await rpc<DeleteEntryRequest, Record<string, never>>("DeleteEntry", req);
}

export async function createEntryWithAttachment(
  date: string,
  type: EntryType,
  content: string,
  source: EntrySource,
  file: File
): Promise<Entry> {
  const buffer = await file.arrayBuffer();
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const base64 = btoa(binary);

  return rpc<Record<string, unknown>, Entry>("CreateEntry", {
    date,
    type,
    content,
    source,
    attachmentData: [base64],
    attachmentFilenames: [file.name],
    attachmentContentTypes: [file.type],
  });
}

// Chat API

export async function listConversations(limit = 50, offset = 0): Promise<ListConversationsResponse> {
  return rpc<{ limit: number; offset: number }, ListConversationsResponse>(
    "ListConversations",
    { limit, offset },
    CHAT_SERVICE
  );
}

export async function getConversation(id: string): Promise<Conversation> {
  return rpc<{ id: string }, Conversation>("GetConversation", { id }, CHAT_SERVICE);
}

// Streaming chat - reads NDJSON response from Connect-go server streaming
export async function* streamChat(
  conversationId: string,
  message: string
): AsyncGenerator<{ content: string; done: boolean; conversationId: string; sources: SourceReference[] }> {
  const resp = await fetch(`${CHAT_SERVICE}/Chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ conversationId, message }),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`Chat failed (${resp.status}): ${text}`);
  }

  const reader = resp.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop()!;
    for (const line of lines) {
      if (!line.trim()) continue;
      const parsed = JSON.parse(line);
      yield parsed.result;
    }
  }
  if (buffer.trim()) {
    const parsed = JSON.parse(buffer);
    yield parsed.result;
  }
}
