import type {
  DailyNote,
  Entry,
  ListDailyNotesResponse,
  CreateEntryRequest,
  DeleteEntryRequest,
  GetDailyNoteRequest,
  ListDailyNotesRequest,
} from "./types";
import { type EntryType, type EntrySource } from "./types";

const SERVICE = "/blackwood.v1.DailyNotesService";

// Connect-go uses POST with JSON body and Content-Type: application/json.
// Field names use camelCase in the JSON wire format (protobuf JSON mapping).
async function rpc<Req, Res>(method: string, request: Req): Promise<Res> {
  const url = `${SERVICE}/${method}`;
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
