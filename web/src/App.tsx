import { useEffect, useState } from "react";
import { api, type Readiness, type VersionInfo } from "./api/client";

const refreshMs = 5000;

// Readiness answers 503 with the same body when the node is not ready.
async function fetchReadiness(): Promise<Readiness | null> {
  const { data, error } = await api.GET("/readyz");
  return data ?? (error as Readiness | undefined) ?? null;
}

export function App() {
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [version, setVersion] = useState<VersionInfo | null>(null);
  const [unreachable, setUnreachable] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [ready, ver] = await Promise.all([fetchReadiness(), api.GET("/api/v1/version")]);
        if (cancelled) return;
        setReadiness(ready);
        setVersion(ver.data ?? null);
        setUnreachable(false);
      } catch {
        if (!cancelled) setUnreachable(true);
      }
    };
    void load();
    const timer = setInterval(load, refreshMs);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, []);

  const status = unreachable ? "unreachable" : readiness?.ready ? "ready" : readiness ? "not-ready" : "loading";
  const label = {
    unreachable: "Hub unreachable",
    ready: "Ready",
    "not-ready": readiness?.shuttingDown ? "Shutting down" : "Not ready",
    loading: "Checking…",
  }[status];

  return (
    <main className="page">
      <header className="header">
        <h1>Home Hub</h1>
        {version && (
          <span className="muted">
            {version.version} · {version.commit}
          </span>
        )}
      </header>

      <section className="card" aria-live="polite">
        <div className="status">
          <span className={`dot dot-${status}`} aria-hidden="true" />
          <strong>{label}</strong>
        </div>
        {readiness && readiness.checks.length > 0 && (
          <ul className="checks">
            {readiness.checks.map((c) => (
              <li key={c.name}>
                <span className={`dot dot-${c.ok ? "ready" : "not-ready"}`} aria-hidden="true" />
                <span className="check-name">{c.name}</span>
                {!c.critical && <span className="tag">non-critical</span>}
                {c.error && <span className="muted check-error">{c.error}</span>}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
