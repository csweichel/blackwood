import { useState, useEffect, useCallback, useRef } from "react";
import { getImportJobs, submitImport, deleteImportJob } from "../api/client";
import type { ImportJobStatus } from "../api/types";

interface UseImportJobsReturn {
  jobs: ImportJobStatus[];
  activeCount: number;
  submit: (files: File[]) => Promise<void>;
  refresh: () => Promise<void>;
  deleteJob: (id: string) => Promise<void>;
}

function isActive(job: ImportJobStatus): boolean {
  return job.status === "pending" || job.status === "processing";
}

export function useImportJobs(): UseImportJobsReturn {
  const [jobs, setJobs] = useState<ImportJobStatus[]>([]);
  const trackedIds = useRef<Set<string>>(new Set());
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stopPolling = useCallback(() => {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const poll = useCallback(async () => {
    try {
      const ids = Array.from(trackedIds.current);
      if (ids.length === 0) return;
      const result = await getImportJobs(ids);
      const fetched = result.jobs ?? [];
      setJobs(fetched);

      // Stop polling if all jobs are terminal
      if (fetched.length > 0 && fetched.every((j) => !isActive(j))) {
        stopPolling();
      }
    } catch {
      // Backend may not exist yet — silently ignore polling errors
    }
  }, [stopPolling]);

  const startPolling = useCallback(() => {
    if (intervalRef.current !== null) return;
    intervalRef.current = setInterval(poll, 2000);
  }, [poll]);

  // On mount, check for any active jobs the server knows about
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const result = await getImportJobs(undefined, true);
        const active = result.jobs ?? [];
        if (cancelled) return;
        if (active.length > 0) {
          for (const j of active) {
            trackedIds.current.add(j.id);
          }
          setJobs(active);
          startPolling();
        }
      } catch {
        // Backend may not exist yet — ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [startPolling]);

  // Cleanup on unmount
  useEffect(() => {
    return () => stopPolling();
  }, [stopPolling]);

  const submit = useCallback(
    async (files: File[]) => {
      try {
        const result = await submitImport(files);
        const newIds = result.jobIds ?? [];
        for (const id of newIds) {
          trackedIds.current.add(id);
        }
        // Immediately poll to get initial statuses
        await poll();
        startPolling();
      } catch {
        // If submit fails, add placeholder error jobs so the UI shows feedback
        const errorJobs: ImportJobStatus[] = files.map((f, i) => ({
          id: `error-${Date.now()}-${i}`,
          status: "error" as const,
          filename: f.name,
          fileType: f.name.endsWith(".note") ? "viwoods" : "obsidian",
          source: "upload",
          progress: 0,
          totalSteps: 0,
          resultJson: JSON.stringify({ error: "Failed to submit import" }),
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        }));
        setJobs((prev) => [...prev, ...errorJobs]);
      }
    },
    [poll, startPolling]
  );

  const deleteJob = useCallback(
    async (id: string) => {
      try {
        await deleteImportJob(id);
      } catch {
        // Ignore errors — remove from local state regardless
      }
      trackedIds.current.delete(id);
      setJobs((prev) => prev.filter((j) => j.id !== id));
    },
    []
  );

  const refresh = useCallback(async () => {
    await poll();
  }, [poll]);

  const activeCount = jobs.filter(isActive).length;

  return { jobs, activeCount, submit, refresh, deleteJob };
}
