import { useEffect, useRef, useState } from "react";
import type { GroupId } from "@/data/polities";
import { METRICS, type MetricId } from "./metrics";
import { SidePanel } from "./SidePanel";
import { WorldMap, type AtlasMapHandle } from "./WorldMap";

export function AtlasApp() {
  const [metric, setMetric] = useState<MetricId>("population");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [hoverId, setHoverId] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState<GroupId | "all">("all");
  const mapRef = useRef<AtlasMapHandle>(null);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelectedId(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const active = METRICS.find((item) => item.id === metric) ?? METRICS[0];

  return (
    <main className="flex h-dvh flex-col bg-parchment text-ink">
      <header className="shrink-0 border-b border-line px-4 py-3 sm:px-5">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <p className="text-xs font-semibold tracking-wide text-brass uppercase">
              CCXX до н. э. · выборка держав
            </p>
            <h1 className="font-display text-4xl leading-none sm:text-5xl">Атлас Архимеда</h1>
            <p className="mt-1 hidden max-w-xl text-sm text-muted sm:block">{active.caption}</p>
          </div>
          <div
            role="tablist"
            aria-label="Метрика раскраски"
            className="flex gap-1 overflow-x-auto rounded-full bg-parchment-2 p-1"
          >
            {METRICS.map((item) => {
              const on = item.id === metric;
              return (
                <button
                  key={item.id}
                  type="button"
                  role="tab"
                  aria-selected={on}
                  onClick={() => setMetric(item.id)}
                  className={
                    "min-h-11 shrink-0 rounded-full px-3 text-sm font-semibold sm:px-4 " +
                    (on ? "bg-cinnabar text-parchment" : "text-ink hover:bg-parchment")
                  }
                >
                  {item.label}
                </button>
              );
            })}
          </div>
        </div>
      </header>

      <div className="grid min-h-0 flex-1 grid-rows-[minmax(18rem,1.15fr)_minmax(16rem,0.9fr)] lg:grid-cols-[minmax(0,1fr)_23rem] lg:grid-rows-1">
        <WorldMap
          ref={mapRef}
          metric={metric}
          selectedId={selectedId}
          onSelect={setSelectedId}
          onHover={setHoverId}
        />
        <SidePanel
          metric={metric}
          selectedId={selectedId}
          hoverId={hoverId}
          query={query}
          group={group}
          onQuery={setQuery}
          onGroup={setGroup}
          onSelect={setSelectedId}
          onFocus={() => {
            if (selectedId) mapRef.current?.focusId(selectedId);
          }}
        />
      </div>
    </main>
  );
}
