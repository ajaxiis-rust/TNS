#!/usr/bin/env python3
"""summarize.py — сводка прогонов llama-bench по папке результатов.

Читает все *.gguf-выгрузки llama-bench (json) и manifest.json в переданной
папке, печатает markdown-сводку: таблицы pp/tg по потокам на модель и
рекомендуемые числа потоков для config.yaml (gen_threads, embed_threads).

Использование: python3 summarize.py <папка-результатов>
"""
from __future__ import annotations

import json
import sys
from collections import defaultdict
from pathlib import Path


def load_runs(run_dir: Path) -> list[dict]:
    rows: list[dict] = []
    for f in sorted(run_dir.glob("*.json")):
        if f.name == "manifest.json":
            continue
        try:
            data = json.loads(f.read_text())
        except json.JSONDecodeError:
            print(f"пропущен битый файл: {f.name}", file=sys.stderr)
            continue
        for row in data if isinstance(data, list) else [data]:
            row["_file"] = f.name
            rows.append(row)
    return rows


def fmt(value: float | None) -> str:
    return f"{value:.1f}" if isinstance(value, (int, float)) else "-"


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    run_dir = Path(sys.argv[1])
    if not run_dir.is_dir():
        print(f"нет папки: {run_dir}", file=sys.stderr)
        return 2

    manifest = {}
    manifest_file = run_dir / "manifest.json"
    if manifest_file.exists():
        manifest = json.loads(manifest_file.read_text())

    rows = load_runs(run_dir)
    if not rows:
        print("нет данных llama-bench в папке результатов", file=sys.stderr)
        return 1

    # модель -> ngl -> n_threads -> (pp_ts, tg_ts)
    by_model: dict[str, dict[str, dict[int, tuple[float, float]]]] = defaultdict(
        lambda: defaultdict(dict)
    )
    meta: dict[str, dict] = {}
    for row in rows:
        model = Path(row.get("model_filename", "?")).name
        ngl = row.get("n_gpu_layers", 0)
        key = f"ngl={ngl}"
        threads = int(row.get("n_threads", 0))
        ts = float(row.get("avg_ts") or 0.0)
        n_prompt = int(row.get("n_prompt") or 0)
        n_gen = int(row.get("n_gen") or 0)
        pp, tg = by_model[model][key].get(threads, (0.0, 0.0))
        if n_prompt > 0 and n_gen == 0:
            pp = ts
        elif n_gen > 0:
            tg = ts
        by_model[model][key][threads] = (pp, tg)
        meta[model] = {
            "type": row.get("model_type", "?"),
            "size_mb": round((row.get("model_size") or 0) / 1e6, 1),
            "role": Path(row.get("_file", "?")).name.split("__")[0],
        }

    out: list[str] = []
    out.append("# Бенч моделей станции (llama-bench)")
    out.append("")

    def num(value: object) -> float:
        try:
            return float(value)  # поля manifest приходят строками из shell
        except (TypeError, ValueError):
            return 0.0

    if manifest:
        out.append(
            f"Дата: {manifest.get('date', '?')}, режим: `{manifest.get('mode', '?')}`, "
            f"потоки-сетка: `{manifest.get('threads', '?')}`, pp/tg: "
            f"{manifest.get('n_prompt', '?')}/{manifest.get('n_gen', '?')}, "
            f"повторов: {manifest.get('reps', '?')}."
        )
        out.append("")
        out.append(
            f"Машина: {manifest.get('cpu_info', '?')} ({manifest.get('nproc', '?')} потоков), "
            f"ОЗУ {round(num(manifest.get('mem_total_kb')) / 1e6, 1)} ГБ, "
            f"модели: `{manifest.get('models_dir', '?')}`."
        )
        out.append("")
        failed = manifest.get("failed") or []
        if failed:
            out.append("**Не замерено (ошибка llama-bench):**")
            for name in failed:
                out.append(f"- `{name}` — см. `llama-bench.log` (файл модели повреждён или не грузится).")
            out.append("")

    recommendations: list[str] = []
    for model in sorted(by_model):
        info = meta.get(model, {})
        out.append(f"## {model}")
        out.append("")
        out.append(
            f"Роль: `{info.get('role', '?')}`, тип: {info.get('type', '?')}, "
            f"файл: {info.get('size_mb', '?')} МБ."
        )
        out.append("")
        for ngl_key in sorted(by_model[model]):
            table = by_model[model][ngl_key]
            out.append(f"### {ngl_key}")
            out.append("")
            out.append("| потоки | pp t/s | tg t/s |")
            out.append("|---|---|---|")
            best_pp = best_tg = (0.0, 0)
            for threads in sorted(table):
                pp, tg = table[threads]
                out.append(f"| {threads} | {fmt(pp)} | {fmt(tg)} |")
                if pp > best_pp[0]:
                    best_pp = (pp, threads)
                if tg > best_tg[0]:
                    best_tg = (tg, threads)
            out.append("")
            role = info.get("role", "")
            if role == "embed" or all(tg == 0 for _, tg in table.values()):
                recommendations.append(
                    f"- `{model}` ({ngl_key}): эмбеддер, лучший pp "
                    f"{fmt(best_pp[0])} t/s на {best_pp[1]} потоках → "
                    f"`embed_threads: {best_pp[1]}`."
                )
            else:
                recommendations.append(
                    f"- `{model}` ({ngl_key}): лучший tg {fmt(best_tg[0])} t/s на "
                    f"{best_tg[1]} потоках (pp {fmt(best_pp[0])} t/s на {best_pp[1]}) → "
                    f"`gen_threads: {best_tg[1]}`."
                )

    out.append("## Рекомендации для config.yaml")
    out.append("")
    out.extend(recommendations or ["- данных недостаточно."])
    out.append("")
    out.append(
        "Примечание: `gen_ctx` на скорость влияет слабо, но держит KV-кэш в "
        "ОЗУ; при утечках памяти смотреть сюда. `gen_parallel: 1` для этой "
        "машины — эталон при tg-замерах выше."
    )
    print("\n".join(out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
