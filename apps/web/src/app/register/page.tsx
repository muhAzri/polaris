"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export default function RegisterPage() {
  const { register } = useAuth();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await register(name, email, password);
      router.push("/courses");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not register");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="frame split py-16">
      <header className="flex flex-col gap-3">
        <h1 className="text-display">Sign up</h1>
        <p className="prose-measure text-lg text-muted">
          New accounts start as students. Ask an admin to promote you to teacher.
        </p>
      </header>

      <div className="flex flex-col gap-8">
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <label className="field">
          <span className="field-label">Name</span>
          <input
            required
            autoComplete="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">Email</span>
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="input"
          />
        </label>
        <label className="field">
          <span className="field-label">Password</span>
          <input
            type="password"
            required
            minLength={8}
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="input"
          />
          <span className="field-hint">At least 8 characters.</span>
        </label>

        {error ? (
          <p role="alert" className="notice">
            {error}
          </p>
        ) : null}

        <button type="submit" disabled={submitting} aria-busy={submitting} className="btn btn-primary">
          {submitting ? "Creating account…" : "Create account"}
        </button>
      </form>

      <p className="border-t border-rule pt-5 text-sm text-muted">
        Already have an account?{" "}
        <Link href="/login" className="link">
          Log in
        </Link>
      </p>
      </div>
    </main>
  );
}
