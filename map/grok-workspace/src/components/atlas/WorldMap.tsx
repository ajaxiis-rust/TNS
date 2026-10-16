import { geoGraticule, geoNaturalEarth1, geoPath } from "d3-geo";
import type { GeoPermissibleObjects } from "d3-geo";
import { select } from "d3-selection";
import { zoom, zoomIdentity, zoomTransform, type ZoomBehavior, type ZoomTransform } from "d3-zoom";
import { Minus, Plus, RotateCcw } from "lucide-react";
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, type ReactNode } from "react";
import landData from "@/data/land.json";
import { MAP_NOTES, polities, polityById, type Polity } from "@/data/polities";
import { metricById, type MetricId } from "./metrics";
import { colorFor } from "./palette";

export const VB_W = 1200;
export const VB_H = 680;

const projection = geoNaturalEarth1();
projection.precision(0.4);
projection.fitExtent(
  [
    [14, 14],
    [VB_W - 14, VB_H - 14],
  ],
  { type: "Sphere" },
);

const pathGen = geoPath(projection);
const sphereD = pathGen({ type: "Sphere" }) ?? "";
const graticuleD = pathGen(geoGraticule().step([20, 20])()) ?? "";
const landD = pathGen(landData as unknown as GeoPermissibleObjects) ?? "";

export type AtlasMapHandle = {
  focusId: (id: string) => void;
  reset: () => void;
};

type Tip = { id: string; x: number; y: number };

type Props = {
  metric: MetricId;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onHover: (id: string | null) => void;
};

function domain(metric: MetricId) {
  let min = Infinity;
  let max = -Infinity;
  for (const polity of polities) {
    const value = polity[metric];
    if (value < min) min = value;
    if (value > max) max = value;
  }
  return { min, max };
}

function frame(x0: number, y0: number, x1: number, y1: number, pad = 0.78) {
  const dx = Math.max(28, x1 - x0);
  const dy = Math.max(28, y1 - y0);
  const k = Math.max(1, Math.min(14, pad / Math.max(dx / VB_W, dy / VB_H)));
  const cx = (x0 + x1) / 2;
  const cy = (y0 + y1) / 2;
  return zoomIdentity.translate(VB_W / 2, VB_H / 2).scale(k).translate(-cx, -cy);
}

function logTick(min: number, max: number, t: number) {
  return Math.exp(Math.log(min) + (Math.log(max) - Math.log(min)) * t);
}

export const WorldMap = forwardRef<AtlasMapHandle, Props>(function WorldMap(
  { metric, selectedId, onSelect, onHover },
  ref,
) {
  const svgRef = useRef<SVGSVGElement>(null);
  const gRef = useRef<SVGGElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const zoomRef = useRef<ZoomBehavior<SVGSVGElement, unknown> | null>(null);
  const onHoverRef = useRef(onHover);
  const animRef = useRef<number | null>(null);
  const [scale, setScale] = useState(1);
  const [tip, setTip] = useState<Tip | null>(null);
  onHoverRef.current = onHover;

  const { min, max } = useMemo(() => domain(metric), [metric]);
  const metricDef = metricById(metric);

  const drawn = useMemo(
    () =>
      polities.map((polity) => {
        const centroid = pathGen.centroid(polity.feature);
        return {
          polity,
          d: pathGen(polity.feature) ?? "",
          area: pathGen.area(polity.feature),
          centroid,
        };
      }),
    [],
  );

  function paintChrome(k: number) {
    const root = gRef.current;
    if (!root) return;
    root.querySelectorAll<SVGTextElement>(".map-label").forEach((el) => {
      const area = Number(el.dataset.area || 0);
      const force = el.dataset.force === "1";
      el.setAttribute("font-size", String(12.5 / k));
      el.setAttribute("stroke-width", String(3.4 / k));
      el.style.display = force || area * k * k > 2600 ? "" : "none";
    });
    root.querySelectorAll<SVGTextElement>(".map-note").forEach((el) => {
      el.setAttribute("font-size", String(11 / k));
      el.style.display = k < 1.65 ? "" : "none";
    });
    root.querySelectorAll<SVGCircleElement>(".capital-mark").forEach((el) => {
      el.setAttribute("r", String(3.3 / k));
    });
    root.querySelectorAll<SVGCircleElement>(".capital-ring").forEach((el) => {
      el.setAttribute("r", String(5.4 / k));
      el.setAttribute("stroke-width", String(1.2 / k));
    });
  }

  function stopFly() {
    if (animRef.current != null) cancelAnimationFrame(animRef.current);
    animRef.current = null;
  }

  function fly(target: ZoomTransform) {
    const svg = svgRef.current;
    const behavior = zoomRef.current;
    if (!svg || !behavior) return;
    stopFly();
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const from = zoomTransform(svg);
    if (reduce) {
      select(svg).call(behavior.transform, target);
      setScale(target.k);
      return;
    }
    const t0 = performance.now();
    const duration = 680;
    const step = (now: number) => {
      const raw = Math.min(1, (now - t0) / duration);
      const e = 1 - (1 - raw) ** 3;
      const k = Math.exp(Math.log(from.k) + (Math.log(target.k) - Math.log(from.k)) * e);
      const x = from.x + (target.x - from.x) * e;
      const y = from.y + (target.y - from.y) * e;
      select(svg).call(behavior.transform, zoomIdentity.translate(x, y).scale(k));
      if (raw < 1) animRef.current = requestAnimationFrame(step);
      else {
        animRef.current = null;
        setScale(target.k);
      }
    };
    animRef.current = requestAnimationFrame(step);
  }

  useEffect(() => {
    const svg = svgRef.current;
    const g = gRef.current;
    if (!svg || !g) return;
    const behavior = zoom<SVGSVGElement, unknown>()
      .scaleExtent([1, 14])
      .clickDistance(6)
      .filter((event) => {
        const target = event.target as Element | null;
        if (target?.closest("[data-map-ui]")) return false;
        return (!event.ctrlKey || event.type === "wheel") && !event.button;
      })
      .on("start", () => {
        setTip(null);
        onHoverRef.current(null);
      })
      .on("zoom", (event) => {
        g.setAttribute("transform", event.transform.toString());
        paintChrome(event.transform.k);
      })
      .on("end", (event) => {
        if (animRef.current == null) setScale(event.transform.k);
      });
    zoomRef.current = behavior;
    select(svg).call(behavior);
    paintChrome(zoomTransform(svg).k || 1);
    return () => {
      stopFly();
      select(svg).on(".zoom", null);
    };
  }, []);

  const flyRef = useRef(fly);
  flyRef.current = fly;

  useImperativeHandle(ref, () => ({
    focusId(id: string) {
      const polity = polityById(id);
      if (!polity) return;
      const [[x0, y0], [x1, y1]] = pathGen.bounds(polity.feature);
      flyRef.current(frame(x0, y0, x1, y1, 0.62));
    },
    reset() {
      flyRef.current(zoomIdentity);
    },
  }));

  function zoomBy(factor: number) {
    const svg = svgRef.current;
    if (!svg) return;
    const current = zoomTransform(svg);
    const nextK = Math.max(1, Math.min(14, current.k * factor));
    const [lx, ly] = current.invert([VB_W / 2, VB_H / 2]);
    fly(zoomIdentity.translate(VB_W / 2, VB_H / 2).scale(nextK).translate(-lx, -ly));
  }

  function focusBox(west: number, south: number, east: number, north: number) {
    const [[x0, y0], [x1, y1]] = pathGen.bounds({
      type: "Polygon",
      coordinates: [
        [
          [west, south],
          [east, south],
          [east, north],
          [west, north],
          [west, south],
        ],
      ],
    });
    fly(frame(x0, y0, x1, y1, 0.9));
  }

  function moveTip(event: { clientX: number; clientY: number }, id: string) {
    const box = wrapRef.current?.getBoundingClientRect();
    if (!box) return;
    setTip({ id, x: event.clientX - box.left, y: event.clientY - box.top });
  }

  const hovered = tip ? polityById(tip.id) : undefined;
  const selected = polityById(selectedId);

  return (
    <div ref={wrapRef} className="relative h-full min-h-0 w-full overflow-hidden bg-sea">
      <svg
        ref={svgRef}
        viewBox={`0 0 ${VB_W} ${VB_H}`}
        className="h-full w-full touch-none"
        role="img"
        aria-label="Карта мира в 220 году до нашей эры"
      >
        <g ref={gRef}>
          <path d={sphereD} className="fill-sea" onClick={() => onSelect(null)} />
          <path d={graticuleD} className="fill-none stroke-parchment/25" strokeWidth={0.6} />
          <path d={landD} className="fill-land" onClick={() => onSelect(null)} />
          <g clipPath="url(#land-clip)">
            {drawn.map(({ polity, d }) => (
              <path
                key={polity.id}
                d={d}
                className={
                  "polity" +
                  (selectedId === polity.id ? " is-selected" : "") +
                  (tip?.id === polity.id ? " is-hover" : "")
                }
                style={{ fill: colorFor(polity[metric], min, max) }}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelect(polity.id);
                }}
                onPointerEnter={() => onHover(polity.id)}
                onPointerMove={(event) => moveTip(event, polity.id)}
                onPointerLeave={() => {
                  onHover(null);
                  setTip(null);
                }}
              >
                <title>{polity.name}</title>
              </path>
            ))}
          </g>
          <path
            d={landD}
            className="pointer-events-none fill-none stroke-ink/45"
            strokeWidth={0.7}
            vectorEffect="non-scaling-stroke"
          />
          {MAP_NOTES.map((note) => {
            const point = projection(note.at);
            if (!point) return null;
            return (
              <text
                key={note.text + note.at.join(",")}
                className="map-note pointer-events-none fill-ink/40 font-sans"
                x={point[0]}
                y={point[1]}
                textAnchor="middle"
                fontSize={11 / scale}
                style={{ display: scale < 1.65 ? undefined : "none" }}
              >
                {note.text}
              </text>
            );
          })}
          {drawn.map(({ polity, area, centroid }) => {
            if (!Number.isFinite(centroid[0]) || !Number.isFinite(centroid[1])) return null;
            const force = selectedId === polity.id || tip?.id === polity.id;
            const visible = force || area * scale * scale > 2600;
            return (
              <text
                key={polity.id}
                className="map-label"
                data-area={area}
                data-force={force ? "1" : "0"}
                x={centroid[0]}
                y={centroid[1]}
                textAnchor="middle"
                fontSize={12.5 / scale}
                strokeWidth={3.4 / scale}
                style={{ display: visible ? undefined : "none" }}
              >
                {polity.label}
              </text>
            );
          })}
          {selected?.capitalAt ? <Capital polity={selected} scale={scale} /> : null}
        </g>
        <defs>
          <clipPath id="land-clip">
            <path d={landD} />
          </clipPath>
        </defs>
      </svg>

      {hovered && tip ? (
        <MapTip
          polity={hovered}
          metric={metric}
          min={min}
          max={max}
          x={tip.x}
          y={tip.y}
          width={wrapRef.current?.clientWidth ?? 800}
          height={wrapRef.current?.clientHeight ?? 500}
        />
      ) : null}

      <div className="pointer-events-none absolute inset-0">
        <div data-map-ui className="pointer-events-auto absolute top-3 left-3 flex rounded-full bg-parchment p-1 shadow-sm">
          <button
            type="button"
            className="min-h-11 rounded-full px-3 text-sm font-semibold text-ink"
            onClick={() => fly(zoomIdentity)}
          >
            Мир
          </button>
          <button
            type="button"
            className="min-h-11 rounded-full px-3 text-sm font-semibold text-cinnabar"
            onClick={() => focusBox(-14, 6, 100, 54)}
          >
            Ойкумена
          </button>
        </div>

        <div className="pointer-events-auto absolute bottom-3 left-3 max-w-[16rem] rounded-xl bg-parchment/95 p-3 text-ink shadow-sm">
          <div className="flex items-baseline justify-between gap-3">
            <p className="text-sm font-semibold">{metricDef.label}</p>
            <p className="text-xs text-muted">лог. шкала</p>
          </div>
          <div className="ramp-bar-x mt-2 h-2.5 rounded-full" />
          <div className="mt-1 flex justify-between text-xs text-muted">
            <span>{metricDef.format(logTick(min, max, 0))}</span>
            <span>{metricDef.format(logTick(min, max, 1))}</span>
          </div>
          <p className="mt-2 hidden text-xs leading-snug text-muted sm:block">{metricDef.caption}</p>
        </div>

        <div data-map-ui className="pointer-events-auto absolute right-3 bottom-3 flex flex-col gap-1">
          <MapButton label="Приблизить" onClick={() => zoomBy(1.4)}>
            <Plus className="size-5" />
          </MapButton>
          <MapButton label="Отдалить" onClick={() => zoomBy(1 / 1.4)}>
            <Minus className="size-5" />
          </MapButton>
          <MapButton label="Весь мир" onClick={() => fly(zoomIdentity)}>
            <RotateCcw className="size-5" />
          </MapButton>
        </div>
      </div>
    </div>
  );
});

function Capital({ polity, scale }: { polity: Polity; scale: number }) {
  if (!polity.capitalAt) return null;
  const point = projection(polity.capitalAt);
  if (!point) return null;
  return (
    <g className="pointer-events-none">
      <circle className="capital-mark fill-ink" cx={point[0]} cy={point[1]} r={3.3 / scale} />
      <circle
        className="capital-ring fill-none stroke-parchment"
        cx={point[0]}
        cy={point[1]}
        r={5.4 / scale}
        strokeWidth={1.2 / scale}
      />
    </g>
  );
}

function MapButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      className="flex size-11 items-center justify-center rounded-lg bg-parchment text-ink shadow-sm"
    >
      {children}
    </button>
  );
}

function MapTip({
  polity,
  metric,
  min,
  max,
  x,
  y,
  width,
  height,
}: {
  polity: Polity;
  metric: MetricId;
  min: number;
  max: number;
  x: number;
  y: number;
  width: number;
  height: number;
}) {
  const def = metricById(metric);
  let left = x + 16;
  let top = y - 8;
  if (left > width - 196) left = x - 196;
  if (top > height - 92) top = height - 96;
  if (top < 8) top = 8;
  return (
    <div
      className="pointer-events-none absolute z-20 w-44 rounded-lg bg-ink px-3 py-2 text-parchment shadow-sm"
      style={{ left, top }}
    >
      <p className="font-display text-xl leading-none">{polity.label}</p>
      <p className="mt-1 flex items-center gap-1.5 text-xs text-brass-soft">
        <span
          className="inline-block size-2 rounded-sm"
          style={{ background: colorFor(polity[metric], min, max) }}
        />
        {def.label}: {def.format(polity[metric])}
      </p>
      <p className="text-xs text-parchment/75">{polity.capital}</p>
    </div>
  );
}
