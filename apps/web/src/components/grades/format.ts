export function fmt(value: number | null | undefined) {
  if (value === null || value === undefined) return "–";
  return String(Math.round(value * 100) / 100);
}

export function pct(value: number | null | undefined) {
  return value === null || value === undefined ? "–" : `${fmt(value)}%`;
}

export const AGGREGATIONS = [
  { value: "mean", label: "Mean of grades" },
  { value: "weighted", label: "Weighted mean" },
  { value: "natural", label: "Natural (sum of points)" },
  { value: "median", label: "Median" },
  { value: "min", label: "Lowest grade" },
  { value: "max", label: "Highest grade" },
  { value: "mode", label: "Mode" },
] as const;
