import Link from "next/link";

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
    <main className="frame flex flex-1 flex-col gap-16 py-16">
      <header className="flex max-w-3xl flex-col gap-6 pb-4">
        <h1 className="text-display">
          An open-source LMS inspired by Moodle, built for the modern stack.
        </h1>
        <p className="prose-measure text-lg text-muted">
          Same idea — courses, enrollment, assignments, grading — rebuilt with a clean UI, a
          low-cost infrastructure footprint, and a developer experience that starts with one
          command.
        </p>
        <div className="flex flex-wrap gap-3">
          <Link href="/courses" className="btn btn-primary">
            My courses
          </Link>
          <Link href="/register" className="btn">
            Create an account
          </Link>
        </div>
      </header>

      <section aria-labelledby="why" className="flex flex-col gap-4">
        <h2 id="why" className="text-2xl">
          How it is built
        </h2>
        <dl className="m-0 border-t-2 border-ink">
          {FEATURES.map((feature) => (
            <div
              key={feature.title}
              className="grid gap-x-8 gap-y-1 border-b border-rule py-5 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]"
            >
              <dt className="font-display text-xl">{feature.title}</dt>
              <dd className="prose-measure text-muted">{feature.body}</dd>
            </div>
          ))}
        </dl>
      </section>

      <section aria-label="Stack and status" className="flex flex-col gap-6 border-t border-rule pt-8">
        <p className="text-sm">
          <span className="meta mr-3">Stack</span>
          {STACK.join(" · ")}
        </p>
        <p role="status" className="flex items-center gap-2 text-sm">
          <span
            className={`inline-block size-2 ${api.ok ? "bg-success" : "bg-danger"}`}
            aria-hidden
          />
          <span>{api.ok ? "API is reachable" : "API is unreachable"}</span>
          <span className="meta break-all normal-case">{api.detail}</span>
        </p>
      </section>
    </main>
  );
}
