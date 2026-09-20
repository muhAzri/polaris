"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function LoginPage() {
  const { login } = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await login(email, password);
      router.push("/courses");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not log in");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="frame split py-16">
      <header className="flex flex-col gap-3">
        <h1 className="text-display">Log in</h1>
        <p className="prose-measure text-lg text-muted">
          Use your Polaris account to reach your courses.
        </p>
      </header>

      <div className="flex flex-col gap-8">
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <label className="field">
          <span className="field-label">Email</span>
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            aria-invalid={error ? true : undefined}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">Password</span>
          <input
            type="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={error ? true : undefined}
            className="input"
          />
        </label>

        {error ? (
          <p role="alert" className="notice">
            {error}
          </p>
        ) : null}

        <button type="submit" disabled={submitting} aria-busy={submitting} className="btn btn-primary">
          {submitting ? "Logging in…" : "Log in"}
        </button>
      </form>

      <p className="border-t border-rule pt-5 text-sm text-muted">
        No account yet?{" "}
        <Link href="/register" className="link">
          Sign up
        </Link>
      </p>
      </div>
    </main>
  );
}
