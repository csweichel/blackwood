interface ImportBannerProps {
  activeCount: number;
  onClick: () => void;
}

export default function ImportBanner({ activeCount, onClick }: ImportBannerProps) {
  if (activeCount === 0) return null;

  return (
    <button
      onClick={onClick}
      className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-accent bg-muted hover:bg-border rounded-full transition-colors"
    >
      <svg
        className="w-3.5 h-3.5 animate-spin text-accent"
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
      <span>
        Importing {activeCount} file{activeCount !== 1 ? "s" : ""}...
      </span>
    </button>
  );
}
