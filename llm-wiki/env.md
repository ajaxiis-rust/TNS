# Environment Variables

All `os.Getenv` / `os.LookupEnv` reads detected statically.

Total: 2 env reads.

| Key | Accessor | Function | File |
|-----|----------|----------|------|
| `LEMMABENCH_GOLDEN` | `os.Getenv` | `findGolden` | engine/internal/lemmarpc/golden_test.go:128 |
| `TNS65_LEMMARU_DICT` | `os.Getenv` | `resolveDict` | engine/cmd/tns65-lemmaru/main.go:130 |

