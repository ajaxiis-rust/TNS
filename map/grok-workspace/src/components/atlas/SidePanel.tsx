import { Crosshair, Search } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { GROUPS, polities, polityById, type GroupId, type Polity } from "@/data/polities";
import { METRICS, metricById, type MetricId } from "./metrics";
import { colorFor } from "./palette";

type Props = {
  metric: MetricId;
  selectedId: string | null;
  hoverId: string | null;
  query: string;
  group: GroupId | "all";
  onQuery: (value: string) => void;
  onGroup: (group: GroupId | "all") => void;
  onSelect: (id: string | null) => void;
  onFocus: () => void;
};

export function SidePanel({
  metric,
  selectedId,
  hoverId,
  query,
  group,
  onQuery,
  onGroup,
  onSelect,
  onFocus,
}: Props) {
  const def = metricById(metric);
  const selected = polityById(selectedId);

  const { min, max } = useMemo(() => {
    let lo = Infinity;
    let hi = -Infinity;
    for (const polity of polities) {
      const value = polity[metric];
      if (value < lo) lo = value;
      if (value > hi) hi = value;
    }
    return { min: lo, max: hi };
  }, [metric]);

  const ranked = useMemo(
    () => polities.slice().sort((a, b) => b[metric] - a[metric]),
    [metric],
  );

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    return ranked.filter((polity) => {
      if (group !== "all" && polity.group !== group) return false;
      if (!q) return true;
      return [polity.name, polity.label, polity.native, polity.capital, polity.ruler]
        .join(" ")
        .toLowerCase()
        .includes(q);
    });
  }, [ranked, query, group]);

  const rank = selected ? ranked.findIndex((polity) => polity.id === selected.id) + 1 : 0;

  return (
    <aside className="flex h-full min-h-0 flex-col bg-parchment text-ink">
      <div className="shrink-0 space-y-3 border-b border-line px-4 py-3">
        <label className="relative block">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" />
          <input
            value={query}
            onChange={(event) => onQuery(event.target.value)}
            placeholder="Держава, столица, правитель"
            className="h-11 w-full rounded-full border border-line bg-parchment-2/70 pr-3 pl-10 text-sm outline-none focus:border-brass"
            type="search"
          />
        </label>
        <div className="flex gap-1 overflow-x-auto pb-1">
          <Chip active={group === "all"} onClick={() => onGroup("all")}>
            Все
          </Chip>
          {GROUPS.map((item) => (
            <Chip key={item.id} active={group === item.id} onClick={() => onGroup(item.id)}>
              {item.label}
            </Chip>
          ))}
        </div>
      </div>

      <div className="scroll-thin min-h-0 flex-1 overflow-y-auto px-4 py-3">
        {selected ? (
          <Detail
            polity={selected}
            metric={metric}
            rank={rank}
            total={polities.length}
            max={max}
            onFocus={onFocus}
            onClear={() => onSelect(null)}
          />
        ) : (
          <Intro onOpenSyracuse={() => onSelect("syracuse")} />
        )}

        <div className="mt-4 mb-2 flex items-baseline justify-between">
          <h2 className="text-sm font-semibold">Выборка</h2>
          <p className="text-xs text-muted">
            {rows.length} из {polities.length} · {def.label.toLowerCase()}
          </p>
        </div>

        {rows.length === 0 ? (
          <p className="py-6 text-sm text-muted">Ничего не найдено. Сбросьте поиск или группу.</p>
        ) : (
          <ol className="space-y-0.5">
            {rows.map((polity) => {
              const active = polity.id === selectedId;
              const hot = polity.id === hoverId;
              return (
                <li key={polity.id}>
                  <button
                    type="button"
                    onClick={() => onSelect(polity.id)}
                    className={
                      "flex min-h-11 w-full items-center gap-3 rounded-lg px-2 py-1.5 text-left " +
                      (active ? "bg-parchment-2" : hot ? "bg-parchment-2/70" : "hover:bg-parchment-2/80")
                    }
                  >
                    <span
                      className="size-3 shrink-0 rounded-sm"
                      style={{ background: colorFor(polity[metric], min, max) }}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold">{polity.name}</span>
                      <span className="block truncate text-xs text-muted">{polity.capital}</span>
                    </span>
                    <span className="shrink-0 text-xs font-semibold tabular-nums">
                      {def.format(polity[metric])}
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
        )}

        <p className="mt-4 text-xs leading-relaxed text-muted">
          Цифры — учебная выборка порядка величин на 220 год до н. э., не перепись и не архив.
          Цвет на карте логарифмический: иначе Маурьи и степь закрасили бы всех остальных одним
          тоном. Земли без заливки в выборку не входят.
        </p>
      </div>
    </aside>
  );
}

function Intro({ onOpenSyracuse }: { onOpenSyracuse: () => void }) {
  return (
    <section>
      <p className="text-xs font-semibold tracking-wide text-brass uppercase">220 до н. э.</p>
      <h2 className="mt-1 font-display text-3xl leading-none text-ink">Мир ещё не римский</h2>
      <p className="mt-2 text-sm leading-relaxed text-muted">
        Архимед в Сиракузах, Гиерон II ещё жив. Карфаген строит новую державу в Испании, Цинь не
        успела стать империей, Маурьи держат Индию уже без Ашоки. Выберите метрику сверху — цвет
        стран сменится, легенда и список тоже.
      </p>
      <button
        type="button"
        onClick={onOpenSyracuse}
        className="mt-3 inline-flex min-h-11 items-center rounded-full bg-cinnabar px-4 text-sm font-semibold text-parchment"
      >
        Открыть Сиракузы
      </button>
    </section>
  );
}

function Detail({
  polity,
  metric,
  rank,
  total,
  max,
  onFocus,
  onClear,
}: {
  polity: Polity;
  metric: MetricId;
  rank: number;
  total: number;
  max: number;
  onFocus: () => void;
  onClear: () => void;
}) {
  const def = metricById(metric);
  const value = polity[metric];
  const pct = Math.max(4, Math.round((value / max) * 100));
  const group = GROUPS.find((item) => item.id === polity.group)?.label;

  return (
    <section>
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-semibold tracking-wide text-brass uppercase">
          {group}
          {polity.outside ? " · вне ойкумены" : ""}
        </p>
        <button type="button" onClick={onClear} className="min-h-11 px-2 text-sm text-muted">
          Сбросить
        </button>
      </div>
      <h2 className="font-display text-4xl leading-none">{polity.name}</h2>
      <p className="mt-1 text-sm text-muted">{polity.native}</p>

      <dl className="mt-3 grid grid-cols-2 gap-2 text-sm">
        <div className="rounded-lg bg-parchment-2/80 px-3 py-2">
          <dt className="text-xs text-muted">Столица</dt>
          <dd className="font-semibold">{polity.capital}</dd>
        </div>
        <div className="rounded-lg bg-parchment-2/80 px-3 py-2">
          <dt className="text-xs text-muted">Власть</dt>
          <dd className="font-semibold">{polity.ruler}</dd>
        </div>
      </dl>

      <div className="mt-3">
        <p className="text-xs text-muted">{def.label}</p>
        <p className="font-display text-4xl leading-none text-cinnabar">{def.format(value)}</p>
        <p className="mt-1 text-xs text-muted">
          {rank} из {total} в выборке
        </p>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-parchment-2">
          <div className="h-full rounded-full bg-cinnabar" style={{ width: `${pct}%` }} />
        </div>
      </div>

      <ul className="mt-3 space-y-1.5">
        {METRICS.filter((item) => item.id !== metric).map((item) => {
          const share = Math.max(3, Math.round((polity[item.id] / Math.max(...polities.map((p) => p[item.id]))) * 100));
          return (
            <li key={item.id} className="grid grid-cols-[5.5rem_1fr_auto] items-center gap-2 text-xs">
              <span className="text-muted">{item.label}</span>
              <span className="h-1.5 overflow-hidden rounded-full bg-parchment-2">
                <span className="block h-full rounded-full bg-brass" style={{ width: `${share}%` }} />
              </span>
              <span className="font-semibold tabular-nums">{item.format(polity[item.id])}</span>
            </li>
          );
        })}
      </ul>

      <p className="mt-3 text-sm leading-relaxed">{polity.summary}</p>

      <button
        type="button"
        onClick={onFocus}
        className="mt-3 inline-flex min-h-11 items-center gap-2 rounded-full bg-ink px-4 text-sm font-semibold text-parchment"
      >
        <Crosshair className="size-4" />
        Приблизить на карте
      </button>
    </section>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "min-h-11 shrink-0 rounded-full px-3 text-sm font-semibold " +
        (active ? "bg-ink text-parchment" : "bg-parchment-2 text-ink")
      }
    >
      {children}
    </button>
  );
}
