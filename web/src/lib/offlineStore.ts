import { openDB, type DBSchema, type IDBPDatabase } from "idb";
import type { EntryType, EntrySource } from "../api/types";

// --- Schema ---

export interface CachedDailyNote {
  date: string;
  content: string;
  updatedAt: number;
}

export interface PendingEntry {
  id?: number;
  date: string;
  type: EntryType;
  content: string;
  source: EntrySource;
  attachmentData?: string[];
  attachmentFilenames?: string[];
  attachmentContentTypes?: string[];
  createdAt: number;
}

export interface PendingContentUpdate {
  date: string;
  content: string;
  updatedAt: number;
}

interface BlackwoodDB extends DBSchema {
  dailyNotes: {
    key: string;
    value: CachedDailyNote;
  };
  pendingEntries: {
    key: number;
    value: PendingEntry;
    indexes: {};
  };
  pendingContentUpdates: {
    key: string;
    value: PendingContentUpdate;
  };
}

// --- Database singleton ---

let dbPromise: Promise<IDBPDatabase<BlackwoodDB>> | null = null;

function getDB(): Promise<IDBPDatabase<BlackwoodDB>> {
  if (!dbPromise) {
    dbPromise = openDB<BlackwoodDB>("blackwood-offline", 1, {
      upgrade(db) {
        if (!db.objectStoreNames.contains("dailyNotes")) {
          db.createObjectStore("dailyNotes", { keyPath: "date" });
        }
        if (!db.objectStoreNames.contains("pendingEntries")) {
          db.createObjectStore("pendingEntries", {
            keyPath: "id",
            autoIncrement: true,
          });
        }
        if (!db.objectStoreNames.contains("pendingContentUpdates")) {
          db.createObjectStore("pendingContentUpdates", { keyPath: "date" });
        }
      },
    });
  }
  return dbPromise;
}

// --- Daily note cache ---

export async function cacheDailyNote(
  date: string,
  content: string
): Promise<void> {
  const db = await getDB();
  await db.put("dailyNotes", { date, content, updatedAt: Date.now() });
}

export async function getCachedDailyNote(
  date: string
): Promise<CachedDailyNote | undefined> {
  const db = await getDB();
  return db.get("dailyNotes", date);
}

// --- Pending entries queue ---

export async function queueEntry(entry: PendingEntry): Promise<number> {
  const db = await getDB();
  // Strip the id so autoIncrement assigns one.
  const { id: _id, ...rest } = entry;
  return db.add("pendingEntries", rest as PendingEntry);
}

export async function getPendingEntries(): Promise<PendingEntry[]> {
  const db = await getDB();
  return db.getAll("pendingEntries");
}

export async function removePendingEntry(id: number): Promise<void> {
  const db = await getDB();
  await db.delete("pendingEntries", id);
}

// --- Pending content updates queue ---

export async function queueContentUpdate(
  date: string,
  content: string
): Promise<void> {
  const db = await getDB();
  // Last-write-wins: overwrite any existing update for the same date.
  await db.put("pendingContentUpdates", {
    date,
    content,
    updatedAt: Date.now(),
  });
}

export async function getPendingContentUpdates(): Promise<
  PendingContentUpdate[]
> {
  const db = await getDB();
  return db.getAll("pendingContentUpdates");
}

export async function removePendingContentUpdate(
  date: string
): Promise<void> {
  const db = await getDB();
  await db.delete("pendingContentUpdates", date);
}

// --- Aggregate count ---

export async function getPendingCount(): Promise<number> {
  const db = await getDB();
  const entries = await db.count("pendingEntries");
  const updates = await db.count("pendingContentUpdates");
  return entries + updates;
}
