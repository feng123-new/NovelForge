# Dependency License Inventory

Generated deterministically from `go list -m -json all` and the license files in the Go module cache. Regenerate with:

```sh
GOWORK=off go mod download all
GOWORK=off go run ./scripts/dependency_license_inventory.go > docs/DEPENDENCY_LICENSES.md
```

| Module | Version | Detected license |
|---|---:|---|
| `github.com/MakeNowJust/heredoc` | `v1.0.0` | MIT |
| `github.com/atotto/clipboard` | `v0.1.4` | BSD-3-Clause |
| `github.com/aymanbagabas/go-osc52/v2` | `v2.0.1` | MIT |
| `github.com/aymanbagabas/go-udiff` | `v0.3.1` | BSD-3-Clause OR MIT |
| `github.com/bits-and-blooms/bitset` | `v1.24.6` | BSD-3-Clause |
| `github.com/charmbracelet/bubbles` | `v1.0.0` | MIT |
| `github.com/charmbracelet/bubbletea` | `v1.3.10` | MIT |
| `github.com/charmbracelet/colorprofile` | `v0.4.1` | MIT |
| `github.com/charmbracelet/harmonica` | `v0.2.0` | MIT |
| `github.com/charmbracelet/lipgloss` | `v1.1.0` | MIT |
| `github.com/charmbracelet/x/ansi` | `v0.11.8` | MIT |
| `github.com/charmbracelet/x/cellbuf` | `v0.0.15` | MIT |
| `github.com/charmbracelet/x/exp/golden` | `v0.0.0-20241011142426-46044092ad91` | MIT |
| `github.com/charmbracelet/x/term` | `v0.2.2` | MIT |
| `github.com/clipperhouse/displaywidth` | `v0.11.0` | MIT |
| `github.com/clipperhouse/stringish` | `v0.1.1` | MIT |
| `github.com/clipperhouse/uax29/v2` | `v2.7.0` | MIT |
| `github.com/davecgh/go-spew` | `v1.1.1` | ISC |
| `github.com/dustin/go-humanize` | `v1.0.1` | MIT |
| `github.com/erikgeiser/coninput` | `v0.0.0-20211004153227-1c3628e74d0f` | MIT |
| `github.com/gofrs/flock` | `v0.13.0` | BSD-3-Clause |
| `github.com/google/pprof` | `v0.0.0-20260802141513-ef3492d7dac3` | Apache-2.0 |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause |
| `github.com/hashicorp/golang-lru/v2` | `v2.0.7` | MPL-2.0 |
| `github.com/kr/pretty` | `v0.3.1` | MIT |
| `github.com/kylelemons/godebug` | `v1.1.0` | Apache-2.0 |
| `github.com/lucasb-eyer/go-colorful` | `v1.4.0` | MIT |
| `github.com/mattn/go-isatty` | `v0.0.24` | MIT |
| `github.com/mattn/go-localereader` | `v0.0.1` | MIT |
| `github.com/mattn/go-runewidth` | `v0.0.24` | MIT |
| `github.com/muesli/ansi` | `v0.0.0-20230316100256-276c6243b2f6` | MIT |
| `github.com/muesli/cancelreader` | `v0.2.2` | MIT |
| `github.com/muesli/termenv` | `v0.16.0` | MIT |
| `github.com/ncruces/go-strftime` | `v1.0.0` | MIT |
| `github.com/pmezard/go-difflib` | `v1.0.0` | BSD-2-Clause |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD-3-Clause |
| `github.com/rivo/uniseg` | `v0.4.7` | MIT |
| `github.com/sahilm/fuzzy` | `v0.1.1` | MIT |
| `github.com/stretchr/testify` | `v1.11.1` | MIT |
| `github.com/voocel/agentcore` | `v1.8.2` | Apache-2.0 |
| `github.com/voocel/litellm` | `v1.8.10` | Apache-2.0 |
| `github.com/xo/terminfo` | `v0.0.0-20220910002029-abceb7e1c41e` | MIT |
| `golang.org/x/exp` | `v0.0.0-20231006140011-7918f672742d` | BSD-3-Clause |
| `golang.org/x/image` | `v0.45.0` | BSD-3-Clause |
| `golang.org/x/mod` | `v0.38.0` | BSD-3-Clause |
| `golang.org/x/sync` | `v0.22.0` | BSD-3-Clause |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause |
| `golang.org/x/text` | `v0.41.0` | BSD-3-Clause |
| `golang.org/x/tools` | `v0.48.0` | BSD-3-Clause |
| `gopkg.in/check.v1` | `v1.0.0-20201130134442-10cb98267c6c` | BSD-2-Clause |
| `gopkg.in/yaml.v3` | `v3.0.1` | Apache-2.0 |
| `modernc.org/cc/v4` | `v4.29.1` | BSD-3-Clause |
| `modernc.org/ccgo/v4` | `v4.34.6` | BSD-3-Clause |
| `modernc.org/fileutil` | `v1.4.0` | BSD-3-Clause |
| `modernc.org/gc/v2` | `v2.6.5` | BSD-3-Clause |
| `modernc.org/gc/v3` | `v3.1.4` | BSD-3-Clause |
| `modernc.org/goabi0` | `v0.2.0` | BSD-3-Clause |
| `modernc.org/libc` | `v1.74.4` | BSD-3-Clause |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause |
| `modernc.org/memory` | `v1.11.0` | BSD-3-Clause |
| `modernc.org/opt` | `v0.2.0` | BSD-3-Clause |
| `modernc.org/sortutil` | `v1.2.1` | BSD-3-Clause |
| `modernc.org/sqlite` | `v1.57.0` | BSD-3-Clause |
| `modernc.org/strutil` | `v1.2.1` | BSD-3-Clause |
| `modernc.org/token` | `v1.1.0` | BSD-3-Clause |
