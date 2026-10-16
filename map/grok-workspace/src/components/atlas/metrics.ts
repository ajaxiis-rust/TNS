export type MetricId = "population" | "area" | "army" | "revenue" | "cities";

export type MetricDef = {
  id: MetricId;
  label: string;
  caption: string;
  format: (n: number) => string;
};

function ruFixed(n: number, digits: number): string {
  return n.toFixed(digits).replace(".", ",");
}

function formatBig(n: number, suffix: string): string {
  const tail = suffix ? ` ${suffix}` : "";
  if (n >= 1_000_000) {
    const v = n / 1_000_000;
    const text = v >= 10 ? String(Math.round(v)) : ruFixed(v, 1);
    return `${text} млн${tail}`;
  }
  if (n >= 1000) {
    const v = n / 1000;
    const text = v >= 10 ? String(Math.round(v)) : ruFixed(v, 1);
    return `${text} тыс.${tail}`;
  }
  return `${Math.round(n)}${tail}`;
}

export const METRICS: MetricDef[] = [
  {
    id: "population",
    label: "Население",
    caption: "Где живут люди: Нил, Ганг, Хуанхэ и большие равнины.",
    format: (n) => formatBig(n, ""),
  },
  {
    id: "area",
    label: "Территория",
    caption: "Степь и пустыня раздувают площадь, не всегда силу.",
    format: (n) => formatBig(n, "км²"),
  },
  {
    id: "army",
    label: "Войско",
    caption: "Полевая армия, а не парадные сотни тысяч из хроник.",
    format: (n) => formatBig(n, ""),
  },
  {
    id: "revenue",
    label: "Казна",
    caption: "Условный годовой доход. Египет Птолемеев всё ещё самый богатый двор.",
    format: (n) => `${n >= 10 ? Math.round(n) : ruFixed(n, 1)} т серебра`,
  },
  {
    id: "cities",
    label: "Города",
    caption: "Крупные стены и столицы. Селевкиды — держава городов.",
    format: (n) => `${n}`,
  },
];

export function metricById(id: MetricId): MetricDef {
  return METRICS.find((m) => m.id === id) ?? METRICS[0];
}
