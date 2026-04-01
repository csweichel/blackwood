import { useState } from "react";
import { usePreferences } from "../hooks/usePreferences";

/**
 * Shows a banner when the browser's timezone differs from the configured one.
 * Offers to update the server-side preference to match the browser.
 */
export default function TimezoneBanner() {
  const { preferences, loading, update, detectedTimezone } = usePreferences();
  const [dismissed, setDismissed] = useState(false);
  const [updating, setUpdating] = useState(false);

  if (loading || dismissed) return null;

  // No mismatch if timezone isn't configured yet, or if they match.
  const configured = preferences.timezone;
  if (!configured || configured === detectedTimezone) return null;

  async function handleAccept() {
    setUpdating(true);
    try {
      await update({ timezone: detectedTimezone });
    } finally {
      setUpdating(false);
    }
  }

  return (
    <div className="bg-accent-subtle border border-accent/30 rounded-lg px-4 py-3 mx-4 mt-3 flex items-start gap-3 text-sm max-w-4xl md:mx-auto">
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-accent mt-0.5 shrink-0">
        <circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" />
      </svg>
      <div className="flex-1">
        <p className="text-foreground">
          Your browser timezone is <span className="font-medium">{detectedTimezone}</span>, but Blackwood is set to <span className="font-medium">{configured}</span>.
        </p>
        <div className="flex gap-2 mt-2">
          <button
            onClick={handleAccept}
            disabled={updating}
            className="px-3 py-1 text-xs font-medium rounded-md bg-accent text-accent-foreground hover:bg-accent-hover transition-colors disabled:opacity-50"
          >
            {updating ? "Updating…" : `Switch to ${detectedTimezone}`}
          </button>
          <button
            onClick={() => setDismissed(true)}
            className="px-3 py-1 text-xs font-medium rounded-md text-muted-foreground hover:text-foreground transition-colors"
          >
            Keep {configured}
          </button>
        </div>
      </div>
    </div>
  );
}
