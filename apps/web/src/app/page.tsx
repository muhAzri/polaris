const STACK = ["Go", "Next.js", "PostgreSQL", "S3-compatible storage"];

const FEATURES = [
  { title: "Modular monolith", body: "One deployable Go binary, organized into clear domain boundaries — extract a service later only if you actually need to." },
  { title: "Object storage from day one", body: "Files never touch Postgres. Presigned URLs mean uploads and downloads bypass the API server entirely." },
  { title: "Self-hostable by default", body: "Runs on a single small VPS via Docker Compose. No Kubernetes required to get started." },
];

async function getApiStatus(): Promise<{ ok: boolean; detail: string }> {
  // API_INTERNAL_URL is container-to-container (e.g. http://api:8080 in
  // docker compose); NEXT_PUBLIC_API_URL is what the browser can reach.
  // This fetch runs on the server, so it needs the internal address.
  const apiUrl =
    process.env.API_INTERNAL_URL ??
    process.env.NEXT_PUBLIC_API_URL ??
    "http://localhost:8080";
  try {
    const res = await fetch(`${apiUrl}/api/v1/health`, { cache: "no-store" });
    if (!res.ok) {
      return { ok: false, detail: `API responded with ${res.status}` };
    }
    return { ok: true, detail: apiUrl };
  } catch {
    return { ok: false, detail: `Could not reach ${apiUrl}` };
  }
}

export default async function Home() {
  const api = await getApiStatus();

  return (
    <div className="flex flex-1 flex-col items-center bg-zinc-50 font-sans dark:bg-black">
      <main className="flex w-full max-w-3xl flex-1 flex-col gap-16 px-6 py-24 sm:px-10">
        <header className="flex flex-col gap-4">
          <span className="text-sm font-medium uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Polaris
          </span>
          <h1 className="text-4xl font-semibold tracking-tight text-black dark:text-zinc-50">
            An open-source LMS inspired by Moodle, built for the modern stack.
          </h1>
          <p className="max-w-xl text-lg leading-8 text-zinc-600 dark:text-zinc-400">
            Same idea — courses, enrollment, assignments, grading — rebuilt
            with a clean UI, a low-cost infrastructure footprint, and a
            developer experience that starts with one command.
          </p>
        </header>

        <section className="flex flex-wrap gap-2">
          {STACK.map((item) => (
            <span
              key={item}
              className="rounded-full border border-black/[.08] px-3 py-1 text-sm text-zinc-700 dark:border-white/[.145] dark:text-zinc-300"
            >
              {item}
            </span>
          ))}
        </section>

        <section className="flex items-center gap-3 rounded-lg border border-black/[.08] px-4 py-3 dark:border-white/[.145]">
          <span
            className={`h-2.5 w-2.5 rounded-full ${api.ok ? "bg-emerald-500" : "bg-red-500"}`}
            aria-hidden
          />
          <span className="text-sm text-zinc-700 dark:text-zinc-300">
            {api.ok ? "API is reachable" : "API is unreachable"}{" "}
            <span className="text-zinc-500 dark:text-zinc-500">
              ({api.detail})
            </span>
          </span>
        </section>

        <section className="grid gap-6 sm:grid-cols-3">
          {FEATURES.map((feature) => (
            <div key={feature.title} className="flex flex-col gap-2">
              <h2 className="text-sm font-semibold text-black dark:text-zinc-50">
                {feature.title}
              </h2>
              <p className="text-sm leading-6 text-zinc-600 dark:text-zinc-400">
                {feature.body}
              </p>
            </div>
          ))}
        </section>
      </main>
    </div>
  );
}
