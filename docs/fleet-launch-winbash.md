# Запуск флота на этой машине (Windows / Git Bash)

Запускать **в Git Bash / MINGW64**, не в PowerShell (петля написана на bash: `while`, `$(...)`, `grep`, `tee`).
Источник: [`fleet-flow.md`](./fleet-flow.md) §Запуск.

## 1. Префлайт — один раз на машине, перед петлёй

```bash
MSYS2_ARG_CONV_EXCL='*' claude -p "/fleet-preflight"
```

При `PREFLIGHT_FAIL` петлю **не** стартовать (разошлись gh/git↔MCP или совпали логины машин).

## 2. Петля (winbash)

```bash
mkdir -p ~/fleet-logs && while out=$(MSYS2_ARG_CONV_EXCL='*' FLEET_AGENT=winbash FLEET_LOG=~/fleet-logs/fleet.log claude -p "/work-cycle" --permission-mode auto --model opus --settings '{"ultracode": true}'); do printf '[%s] ⟵ %s\n' "$(date +%H:%M:%S)" "$out" | tee -a ~/fleet-logs/fleet.log; echo "$out" | grep -q "CYCLE_ERROR" && { printf '[%s] ⚠ CYCLE_ERROR — остановка машины\n' "$(date +%H:%M:%S)" | tee -a ~/fleet-logs/fleet.log; break; }; echo "$out" | grep -q "WORK_QUEUE_EMPTY" && break; done
```

## Критично именно для этой машины

- `MSYS2_ARG_CONV_EXCL='*'` — обязателен. Без него POSIX-конверсия превратит `/work-cycle` в `C:/Program Files/Git/work-cycle`. Не комбинировать с `//work-cycle` (двойной слеш).
- `FLEET_AGENT=winbash` — метка машины в логе; на двух других машинах она должна быть другой (`ubuntu` / `wsl`).
- `hostname -s` под Git Bash падает — поэтому `FLEET_AGENT` задан явно.
- Аккаунт `gh auth` / git / MCP на этой машине должен отличаться от двух других, иначе ломается merge-gate «approval ≠ author».

## Порядок конвейера

`/feeder docs/` (человек, разово) → ручной триаж (`owner-agreed`) → `fleet-preflight` (раз на машину) → петля.
Не запускать **до фазы C** (настоящий CI + required checks в ruleset). Стоп всего флота — лейбл `fleet-stop` на любой issue.

## Логи

```bash
tail -f ~/fleet-logs/fleet.log
```

Дешевле, чем opus+ultracode: `--effort xhigh` вместо `--settings '{"ultracode": true}'`.
