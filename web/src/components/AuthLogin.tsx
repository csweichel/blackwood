import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { login } from "../api/auth";
import Logo from "./Logo";

export default function AuthLogin() {
  const navigate = useNavigate();
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      const result = await login(code);
      if (result.ok) {
        navigate("/", { replace: true });
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
        <h1 className="text-lg font-semibold text-foreground mb-1">Welcome back</h1>
        <p className="text-sm text-muted-foreground mb-6">
          Enter the 6-digit code from your authenticator app.
        </p>

        {error && (
          <div className="bg-destructive/10 border border-destructive/20 text-destructive rounded-lg px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <label htmlFor="code" className="block text-sm text-foreground mb-1.5">
            Authentication Code
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
            {submitting ? "Logging in…" : "Login"}
          </button>
        </form>
      </div>
    </div>
  );
}
