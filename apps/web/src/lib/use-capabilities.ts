"use client";

import { useEffect, useState } from "react";

import { getMyCapabilities } from "@/lib/api";

/**
 * Loads what the signed-in user may do in a course. Pages use it to decide
 * which controls to show; the API enforces the same checks, so this only
 * keeps the UI honest.
 */
export function useCourseCapabilities(token: string | null, courseId: string) {
  const [caps, setCaps] = useState<Set<string> | null>(null);

  useEffect(() => {
    if (!token) return;
    getMyCapabilities(token, courseId)
      .then((list) => setCaps(new Set(list)))
      .catch(() => setCaps(new Set()));
  }, [token, courseId]);

  return {
    ready: caps !== null,
    can: (capability: string) => caps?.has(capability) ?? false,
  };
}
