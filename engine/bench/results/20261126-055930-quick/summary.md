# Бенч моделей станции (llama-bench)

Дата: 2026-11-26T05:59:30+02:00, режим: `quick`, потоки-сетка: `1,2,3,4,6,8`, pp/tg: 256/32, повторов: 2.

Машина: AMD Ryzen 5 3500U with Radeon Vega Mobile Gfx (8 потоков), ОЗУ 13.2 ГБ, модели: `/home/ajaxii/PRJ/TNS-GOLD/world_db/models`.

**Не замерено (ошибка llama-bench):**
- `weaver__Katarau-9B-Q4_K_M__ngl0` — см. `llama-bench.log` (файл модели повреждён или не грузится).

## KAINE-Qwen3.5-4B-abliterated.Q4_K_M.gguf

Роль: `weaver`, тип: qwen35 4B Q4_K - Medium, файл: 2772.5 МБ.

### ngl=0

| потоки | pp t/s | tg t/s |
|---|---|---|
| 1 | 28.1 | 1.8 |
| 2 | 34.0 | 2.4 |
| 3 | 34.3 | 3.0 |
| 4 | 34.6 | 3.4 |
| 6 | 33.0 | 3.3 |
| 8 | 33.5 | 2.3 |

## Qwen2.5-3B-Instruct-Q4_K_M.gguf

Роль: `analysis`, тип: qwen2 3B Q4_K - Medium, файл: 1923.9 МБ.

### ngl=0

| потоки | pp t/s | tg t/s |
|---|---|---|
| 1 | 38.9 | 2.3 |
| 2 | 41.2 | 3.4 |
| 3 | 38.2 | 4.4 |
| 4 | 39.7 | 4.9 |
| 6 | 38.3 | 4.5 |
| 8 | 37.0 | 2.1 |

## bge-m3-Q4_K_M.gguf

Роль: `embed`, тип: bert 335M Q4_K - Medium, файл: 430.9 МБ.

### ngl=0

| потоки | pp t/s | tg t/s |
|---|---|---|
| 1 | 167.6 | 0.0 |
| 2 | 128.3 | 0.0 |
| 3 | 212.2 | 0.0 |
| 4 | 175.6 | 0.0 |
| 6 | 131.7 | 0.0 |
| 8 | 198.1 | 0.0 |

## Рекомендации для config.yaml

- `KAINE-Qwen3.5-4B-abliterated.Q4_K_M.gguf` (ngl=0): лучший tg 3.4 t/s на 4 потоках (pp 34.6 t/s на 4) → `gen_threads: 4`.
- `Qwen2.5-3B-Instruct-Q4_K_M.gguf` (ngl=0): лучший tg 4.9 t/s на 4 потоках (pp 41.2 t/s на 2) → `gen_threads: 4`.
- `bge-m3-Q4_K_M.gguf` (ngl=0): эмбеддер, лучший pp 212.2 t/s на 3 потоках → `embed_threads: 3`.

Примечание: `gen_ctx` на скорость влияет слабо, но держит KV-кэш в ОЗУ; при утечках памяти смотреть сюда. `gen_parallel: 1` для этой машины — эталон при tg-замерах выше.
