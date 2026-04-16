interface ConflictBannerProps {
  /** Section names that have conflicts (e.g. ["# Notes"]). */
  conflicts: string[];
  /** Called when the user chooses to keep their local version. */
  onKeepLocal: () => void;
  /** Called when the user chooses to accept the server version. */
  onUseServer: () => void;
}

export default function ConflictBanner({
  conflicts,
  onKeepLocal,
  onUseServer,
}: ConflictBannerProps) {
  if (conflicts.length === 0) return null;

  const sectionList = conflicts
    .map((c) => c.replace(/^# /, ""))
    .join(", ");

  return (
    <div className="bg-muted border border-destructive/30 rounded-lg p-3 mb-4 text-sm">
      <div className="flex items-start gap-2">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="text-destructive shrink-0 mt-0.5"
        >
          <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z" />
          <line x1="12" x2="12" y1="9" y2="13" />
          <line x1="12" x2="12.01" y1="17" y2="17" />
        </svg>
        <div className="flex-1 min-w-0">
          <p className="text-foreground font-medium">
            Conflicting changes in: {sectionList}
          </p>
          <p className="text-muted-foreground mt-1">
            The server updated the same section you were editing. Which version do you want to keep?
          </p>
          <div className="flex gap-2 mt-2">
            <button
              onClick={onKeepLocal}
              className="px-3 py-1 text-xs font-medium text-foreground bg-card border border-border rounded-md hover:bg-muted transition-colors"
            >
              Keep my version
            </button>
            <button
              onClick={onUseServer}
              className="px-3 py-1 text-xs font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors"
            >
              Use server version
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
