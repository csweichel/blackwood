import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { getSetupInfo, submitSetup } from "../api/auth";
import Logo from "./Logo";

export default function AuthSetup() {
  const navigate = useNavigate();
  const [secret, setSecret] = useState("");
  const [qrCode, setQrCode] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    getSetupInfo()
      .then((data) => {
        setSecret(data.secret);
        setQrCode(data.qrCode);
        setLoading(false);
      })
      .catch(() => {
        setError("Failed to load setup info.");
        setLoading(false);
      });
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      const result = await submitSetup(secret, code);
      if (result.ok) {
        navigate("/auth/login");
      } else {
        setError(result.error || "Invalid code. Please try again.");
        setCode("");
      }
    } catch {
      setError("Something went wrong. Please try again.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen bg-background">
      <div className="bg-card border border-border rounded-xl p-8 max-w-sm w-full mx-4">
        <div className="flex justify-center mb-6">
          <Logo height={32} />
        </div>
        <h1 className="text-lg font-semibold text-foreground mb-1">Setup Authenticator</h1>
        <p className="text-sm text-muted-foreground mb-6 leading-relaxed">
          Scan the QR code with your authenticator app, then enter the 6-digit code to confirm.
        </p>

        {error && (
          <div className="bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        {loading ? (
          <div className="text-center text-muted-foreground py-8">Loading…</div>
        ) : (
          <>
            <div className="flex justify-center mb-4">
              <img
                src={`data:image/png;base64,${qrCode}`}
                alt="TOTP QR Code"
                width={200}
                height={200}
                className="rounded-lg"
              />
            </div>

            <div className="bg-muted rounded-md px-3 py-2 font-mono text-xs text-center tracking-wider text-foreground mb-6 break-all select-all">
              {secret}
            </div>

            <form onSubmit={handleSubmit}>
              <label htmlFor="code" className="block text-sm text-foreground mb-1.5">
                Verification Code
              </label>
              <input
                type="text"
                id="code"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                maxLength={6}
                pattern="[0-9]{6}"
                inputMode="numeric"
                autoComplete="one-time-code"
                autoFocus
                required
                className="w-full px-3 py-2 border border-border rounded-lg bg-muted text-foreground text-center text-lg tracking-[0.5em] outline-none focus:border-accent"
              />
              <button
                type="submit"
                disabled={submitting || code.length !== 6}
                className="w-full mt-4 px-3 py-2 bg-accent text-accent-foreground rounded-lg text-sm font-semibold hover:bg-accent-hover disabled:opacity-50 transition-colors"
              >
                {submitting ? "Verifying…" : "Verify & Enable"}
              </button>
            </form>
          </>
        )}
      </div>
    </div>
  );
}
