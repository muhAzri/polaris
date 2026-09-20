"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { useAuth } from "@/lib/auth-context";
import { CalendarView } from "@/components/calendar/CalendarView";

export default function CalendarPage() {
  const { user, token, loading } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!loading && !user) router.push("/login");
  }, [loading, user, router]);

  if (loading || !user || !token) return null;

  return (
    <main className="frame flex flex-1 flex-col gap-10 py-12">
      <header className="flex flex-col gap-2">
        <h1 className="text-display">Calendar</h1>
        <p className="prose-measure text-lg text-muted">
          Your events, your courses&apos; events and site-wide dates in one place.
        </p>
      </header>
      <CalendarView token={token} canCreate />
    </main>
  );
}
