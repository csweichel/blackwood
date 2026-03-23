import { useEffect, useRef } from "react";

export interface ImportFileResult {
  id?: string;
  filename: string;
  status: "pending" | "processing" | "done" | "error";
  message?: string;
  date?: string;
  pagesProcessed?: number;
  source?: string;
}

interface ImportModalProps {
  open: boolean;
  files: ImportFileResult[];
  onClose: () => void;
  onNavigateToDate?: (date: string) => void;
  onDeleteJob?: (id: string) => void;
}

function StatusIcon({ status }: { status: ImportFileResult["status"] }) {
  switch (status) {
    case "pending":
      return (
        <span className="inline-block w-4 h-4 rounded-full bg-muted shrink-0" />
      );
    case "processing":
      return (
        <svg
          className="w-4 h-4 shrink-0 animate-spin text-accent"
          viewBox="0 0 24 24"
          fill="none"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
          />
        </svg>
      );
    case "done":
      return (
        <svg
          className="w-4 h-4 shrink-0 text-accent"
          viewBox="0 0 20 20"
          fill="currentColor"
        >
          <path
            fillRule="evenodd"
            d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
            clipRule="evenodd"
          />
        </svg>
      );
    case "error":
      return (
        <svg
          className="w-4 h-4 shrink-0 text-destructive"
          viewBox="0 0 20 20"
          fill="currentColor"
        >
          <path
            fillRule="evenodd"
            d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
            clipRule="evenodd"
          />
        </svg>
      );
  }
}

export default function ImportModal({
  open,
  files,
  onClose,
  onNavigateToDate,
  onDeleteJob,
}: ImportModalProps) {
  const backdropRef = useRef<HTMLDivElement>(null);

  const isProcessing = files.some(
    (f) => f.status === "pending" || f.status === "processing"
  );
  const doneCount = files.filter(
    (f) => f.status === "done" || f.status === "error"
  ).length;

  // Find the first navigable date from results
  const navigableDate = files.find((f) => f.date)?.date;

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      ref={backdropRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 transition-opacity"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div className="bg-card rounded-lg shadow-xl w-full max-w-md mx-4 overflow-hidden animate-in fade-in">
        {/* Header */}
        <div className="px-5 pt-5 pb-3 flex items-start justify-between">
          <div>
            <h2 className="text-lg font-semibold text-foreground">
              {isProcessing ? "Importing files..." : "Import complete"}
            </h2>
            {isProcessing && files.length > 1 && (
              <p className="text-sm text-muted-foreground mt-1">
                {doneCount} of {files.length} files
              </p>
            )}
          </div>
          <button
            onClick={onClose}
            className="p-1 -mr-1 -mt-1 text-muted-foreground hover:text-foreground rounded transition-colors"
            aria-label="Close"
          >
            <svg className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
              <path
                fillRule="evenodd"
                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                clipRule="evenodd"
              />
            </svg>
          </button>
        </div>

        {/* File list */}
        <div className="px-5 pb-4 max-h-64 overflow-y-auto">
          <ul className="space-y-2">
            {files.map((file, i) => (
              <li key={file.id ?? i} className="flex items-start gap-2.5">
                <div className="mt-0.5">
                  <StatusIcon status={file.status} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <p className="text-sm font-medium text-foreground truncate">
                      {file.filename}
                    </p>
                    {file.source === "watcher" && (
                      <span className="shrink-0 text-[10px] font-medium text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                        auto-imported
                      </span>
                    )}
                  </div>
                  {file.status === "processing" && (
                    <p className="text-xs text-accent">
                      {file.message || "Processing..."}
                    </p>
                  )}
                  {file.status === "done" && file.message && (
                    <p className="text-xs text-muted-foreground">{file.message}</p>
                  )}
                  {file.status === "error" && file.message && (
                    <p className="text-xs text-destructive">{file.message}</p>
                  )}
                </div>
                {onDeleteJob && file.id && file.status !== "done" && (
                  <button
                    onClick={() => onDeleteJob(file.id!)}
                    className="mt-0.5 p-0.5 shrink-0 text-muted-foreground hover:text-destructive rounded transition-colors"
                    aria-label={`Remove ${file.filename}`}
                  >
                    <svg className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                      <path
                        fillRule="evenodd"
                        d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                        clipRule="evenodd"
                      />
                    </svg>
                  </button>
                )}
              </li>
            ))}
          </ul>
        </div>

        {/* Footer */}
        {!isProcessing && (
          <div className="px-5 py-3 bg-muted border-t border-border flex justify-end gap-2">
            {navigableDate && onNavigateToDate && (
              <button
                onClick={() => onNavigateToDate(navigableDate)}
                className="px-3 py-1.5 text-sm font-medium text-primary-foreground bg-primary hover:opacity-90 rounded-lg transition-colors"
              >
                View notes
              </button>
            )}
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground bg-card border border-border hover:bg-muted rounded-lg transition-colors"
            >
              Close
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
