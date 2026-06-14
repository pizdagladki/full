# Автономный флот разработки — описание флоу

Как работает флот Claude Code на репозитории `pizdagladki/full`: компоненты, путь задачи от
спецификации до мержа, гейт проверки и жизненный цикл контекста воркеров.


## Коротко

Три машины, на каждой свой GitHub-аккаунт, по запросу запускают петлю `claude -p "/work-cycle"`.
Один прогон петли = один цикл с чистым контекстом: воркер берёт одну единицу работы из очереди
GitHub, выполняет, печатает результат, завершает процесс. Состояние живёт в GitHub (лейблы,
assignee, PR, комментарии), а не в памяти агента. Очередь наполняет человек: `/feeder` создаёт
задачи, человек одобряет их лейблом `owner-agreed`.


## Принципы

Состояние живёт в GitHub, не в памяти. Любое решение, которое должно пережить цикл, пишется в
GitHub: лейбл, assignee, комментарий ревью, счётчик раундов. GitHub — общая шина состояния для
трёх машин.

Каждый цикл — чистый контекст. `claude -p "/work-cycle"` это новый процесс с пустым окном. Делает
одну единицу работы и умирает. Следующий цикл — снова пустое окно.

Координация через GitHub, не через прямое общение. Машины не общаются напрямую — только читают и
пишут состояние в GitHub.


## Компоненты

`.claude/` (в гите, общая на команду):
- `settings.json` — permissions (allow/deny) и регистрация хуков.
- `agents/coder.md` — исполнитель (Sonnet): пишет код и тесты в worktree, гоняет make-гейты,
  коммитит локально; GitHub/PR/борд не трогает.
- `agents/code-reviewer.md`, `agents/security-reviewer.md` — read-only ревьюеры (Opus,
  `tools: Read, Grep, Glob`).
- `hooks/gofmt.mjs` — gofmt на .go после правки.
- `hooks/block-github.mjs` — блокирует запись в `.github/`.
- `hooks/trace.mjs` — пишет строку в лог на каждое действие.
- `hooks/spellcheck.mjs` — сообщает опечатки (не блокирует).
- `skills/work-cycle/` — оркестратор: `SKILL.md` + `steps/{select,implement,review,address}.md`.
- `skills/fleet-preflight/` — разовый стартовый чек машины: identity (gh/git == MCP и различие трёх
  аккаунтов), доступность MCP, гейт-пререквизиты. Запускать перед петлёй.
- `skills/feeder/` — спецификация в backlog задач.
- `skills/go-backend-conventions/`, `skills/frontend-conventions/`, `skills/review-pr/` — канон (знание).
- `skills/new-service/`, `skills/new-resource/` — скаффолдинг.

`.github/` (зона человека, флот сюда не пишет):
- `workflows/ci.yml` — джобы lint / typecheck / test / build / frontend / spell / spell-diff.
- `workflows/project-in-progress.yml` — двигает карточку в In Progress по claim.
- `CODEOWNERS`, `ISSUE_TEMPLATE/task.md`, `pull_request_template.md`.
- `scripts/spell-diff.sh` — cspell (англ.+рус.) по изменённым строкам для джобы `spell-diff`.

Защита `.github/`: хук `block-github.mjs` + deny-правило в `settings.json` + CODEOWNERS.

Транспорт (чем что делается):
- issue/PR (get/create/update, комментарии, лейблы, assignee, ревью) — GitHub MCP (`mcp__github__*`).
- движение карточек по борде — автоматика (GitHub Actions + встроенные Project-воркфлоу); шаги
  work-cycle борд не трогают. Исключение: feeder кладёт новый issue на борд через
  `gh project item-add` (Bash; `gh project:*` разрешён в `settings.json`).
- включить auto-merge — `gh pr merge <N> --auto --squash` (Bash).
- резолв Copilot-тредов — `gh api graphql ... resolveReviewThread` (Bash).
- локальный код (git worktree/rebase/push, make, gofmt, typos) — Bash.

MCP merge-инструмент не используется нигде — он мержит немедленно, в обход «ждать зелёного CI».


## Лейблы

Девять лейблов — это всё координационное состояние системы. Воркеры читают их каждый цикл.

- `task` — задача для флота (вешает feeder, всегда с `proposed`).
- `owner-agreed` — задача допущена в очередь (вешает человек на триаже).
- `proposed` — создано feeder, человек ещё не смотрел.
- `needs-work` — по PR запрошены правки, доработать (вешает review.md).
- `needs-human` — эскалация, флот сам не вытащит.
- `round-1` / `round-2` / `round-3` — счётчик раундов доработки PR.
- `fleet-stop` — аварийный рубильник всего флота (вешает человек).


## Конвейер: от спеки до мержа

Последовательность:

1. Спецификация лежит в `docs/specs/` (overview, user-flows, флоу-доки, tech-stack).
2. Человек запускает `/feeder docs/` (разово). Feeder режет спеку на мелкие PR-размерные задачи,
   строит граф зависимостей (`Depends on #N`) и создаёт issues с лейблами `task,proposed`.
3. Человек делает триаж: смотрит `proposed`, на годные вешает `owner-agreed` и снимает `proposed`
   в порядке зависимостей (сначала корневые: scaffold, migration, auth); негодные закрывает.
4. Флот (`/work-cycle`) на трёх машинах дренирует очередь: берёт задачи с `task + owner-agreed`,
   у которых все блокеры закрыты.
5. Цикл реализации: implement → PR → review → (при правках address ↔ review) → approve + auto-merge.
6. Мерж закрывает issue (`Closes #N`) и разблокирует зависимые задачи — флот берёт их следующими.

Про feeder: он идемпотентен. В тело каждой задачи прячется отпечаток `<!-- fdr-<area>-<short-slug> -->`,
и перед созданием feeder ищет его среди всех issues (любого статуса) — повторный прогон не
воссоздаёт ни одобренную задачу (с неё снят `proposed`), ни закрытую. Ручные prerequisite (OAuth,
Stripe, инфра) feeder задачами не делает — только помечает `Manual prerequisite (human): …` в
Context.

Триаж — ручной гейт. Без `owner-agreed` задача невидима для флота.


## Один цикл /work-cycle

Оркестратор `work-cycle/SKILL.md` читает `steps/select.md`, выбирает одну единицу работы, запускает
один из `steps/{implement,review,address}.md` по типу, печатает маркеры и завершается.
`disable-model-invocation: true` — запускается только обёрткой.

### select.md — выбор одной единицы

Шаг 0, KILL-SWITCH: если есть открытая issue с `fleet-stop` — печать `WORK_QUEUE_EMPTY`, выход.

Дальше приоритетный каскад (закончить важнее, чем начать), берётся первый подходящий тип:

1. CHANGES — PR с `needs-work`, где ты не последний пушивший. Если `round-N ≥ 3` — поставить
   `needs-human`, снять `needs-work`, пропустить. Иначе тип address.
2. REVIEW — открытый PR, где ты не автор и не последний пушивший, claimable. Иначе тип review.
3. NEW ISSUE — open issue с обоими лейблами `task` и `owner-agreed`, claimable, все блокеры
   `Depends on #X` закрыты. Без `owner-agreed` issue игнорируется.

Claim — эксклюзивная лиза, не лок: назначить себя + штамп-коммент `🤖 [CLAIM] <машина> <ts>`, задержка
1–3 с, перечитать и продолжить ТОЛЬКО если assignees == ровно [ты] (это множество — «ты в списке»
недостаточно), иначе уступить. Claimable = нет assignee, либо протухший claim (последний `[CLAIM]`
старше 30 мин и с тех пор нет прогресса), либо твоя задача от упавшего цикла — так флот переподбирает
осиротевшую крашем работу.

Deadlock breaker: если `task+owner-agreed` issues есть, но все заблокированы (или цикл A↔B) и нет
другой работы — поставить `needs-human` на старейшую, печать `WORK_QUEUE_EMPTY`, выход.

No eligible reviewer: если осталось только ревью PR, где ты автор/последний пушивший (ревьюить
нельзя), и другой работы нет — печать `[SELECT] no-eligible-reviewer` + `WORK_QUEUE_EMPTY` (ревью
требует, чтобы работали все три аккаунта).

Ничего не найдено — `WORK_QUEUE_EMPTY`, выход. Токен `WORK_QUEUE_EMPTY` печатается в каждой точке
выхода (на ошибке вместо него — `CYCLE_ERROR`, см. §Запуск) — по нему обёртка останавливает петлю.

### implement.md — issue в PR

Воркер здесь — оркестратор: код пишет не он, а субагент `coder` (Sonnet).

1. Прочитать issue. Если критерии непроверяемы/неясны — `needs-human`, выход (не гадать).
2. `git fetch`; worktree с веткой `feat/N-<slug>` от свежего `origin/main`.
3. План (мозг): для нетривиальной задачи — плэн-мод и набросок плана (без человеческого аппрува);
   для тривиальной хватит однострочника. Сам воркер код не пишет.
4. Делегировать реализацию субагенту `coder`: дать ему путь worktree, зону из «Service / area»,
   критерии и план. Он пишет код строго в зоне (канон подтягивается сам, новый сервис/ресурс —
   скиллами), table-driven тесты на каждый критерий, моки через mockgen, и коммитит в worktree.
5. `coder` гоняет `make test`, `make cover` (≥80%), `make lint` до зелёного и чинит опечатки (хук +
   CI-джобы spell/spell-diff). Он возвращает фактический процент покрытия (строка `total: … %`);
   воркер проверяет ≥80% сам — голому «green» не верить. Не вышло — вернуть конкретный пробел.
6. SELF-REVIEW: `git diff origin/main...HEAD` из worktree, делегировать дифф + критерии обоим
   ревьюерам (`code-reviewer` + `security-reviewer`); найденное чинить, снова делегируя `coder`.
   Gate: если блокер так и не устранён — `needs-human` и выход БЕЗ открытия PR.
7. Запушить ветку, открыть PR (`Closes #N`, вывод тестов, заполненный PR-шаблон с отмеченными
   чекбоксами). Если push прошёл, а `create_pull_request` упал — снять себя из assignee issue (чтобы
   та не залипла с orphan-веткой), `CYCLE_ERROR pr-create-failed`, выход.
8. Выход. Не мержить — это делает ревьюер.

### review.md — PR в merge или needs-work

1. `git fetch`, сверить актуальный head. Забрать дифф + метаданные PR через MCP. Проверить CI — не
   агрегатный «green», а что КАЖДАЯ ожидаемая джоба (lint/typecheck/test/build/frontend/spell/
   spell-diff) = success на текущем head.sha; skipped/cancelled/устаревшая = провал.
2. Copilot: через GraphQL получить ревью-треды, отделить авторства бота Copilot (id + текст +
   файл/строка). Если ревью Copilot ещё не пришло — отметить и не блокироваться.
3. Делегировать дифф + критерии + Copilot-комментарии обоим ревьюерам. Для каждого Copilot-
   комментария code-reviewer выносит apply (реальный дефект) или dismiss (стиль/ложное/вне scope)
   с одной строкой обоснования.
4. Резолв всех Copilot-тредов (родителем, через GraphQL, в обеих ветках — резолв не равно
   применение): apply — ответ «учтено» + resolve; dismiss — причина + resolve.
5. Вердикт:
    - GOOD = нет блокеров, нет apply-находок Copilot, все ожидаемые CI-джобы = success. Approve через MCP +
      `gh pr merge <N> --auto --squash`.
    - BAD = есть блокер или хоть одна apply-находка. Построчные комментарии (включая применённые
      Copilot-находки своими словами), лейбл `needs-work`, бамп раунда (нет `round-*` → `round-1`;
      иначе `round-N` → `round-(N+1)`), снять себя из assignee (PR возвращается в пул на ревью).
      При раунде ≥3 — `needs-human` вместо `needs-work`.

### address.md — доработать PR

Как и в implement.md, правки пишет субагент `coder`, не сам воркер.

1. Забрать PR + все комментарии ревью через MCP. Это единственный контекст. Отдельно гоняться за
   Copilot-тредами не нужно — ревьюер их уже адъюдицировал и зарезолвил.
2. Сначала синк: `git fetch`; worktree на ветке PR, сброшенный к её последнему удалённому head
   (`git worktree add ../<branch> <branch>` + `git reset --hard origin/<branch>`). Если отстаёт от
   `origin/main` — rebase.
3. Делегировать правки субагенту `coder`: дать ему worktree, зону и комментарии ревьюера. Он правит
   строго по комментариям (без расширения scope) и коммитит; код руками не править.
4. `coder` держит `make test`, `make cover` (≥80%), `make lint` зелёными — воркер лишь подтверждает.
5. Резолв комментариев, push.
6. Снять `needs-work` — PR возвращается на ревью. Счётчик раундов не трогать. Борд не трогать.
7. Выход.


## Пошаговый план вызовов (агенты и скиллы)

Сплошная цепочка от обёртки до мержа. **Жирным** — субагенты (отдельное окно), `моноширинным` —
скиллы, шаги, инструменты и MCP-вызовы. Скиллы `work-cycle` и `feeder` — `disable-model-invocation`
(запускаются только обёрткой / человеком).

### Наполнение очереди (человек, разово)

1. `/feeder docs/` → скилл `feeder`, один прогон: читает `docs/specs/**`, `docs/architecture.md`,
   `.github/ISSUE_TEMPLATE/task.md` и канон-скиллы `go-backend-conventions` / `new-service` /
   `new-resource` / `frontend-conventions`; строит DAG в памяти; на каждую единицу
   `mcp__github__search_issues` (отпечаток `fdr-<area>-<short-slug>`) → если нет,
   `mcp__github__create_issue` (`task,proposed`) → `gh project item-add`.
2. Человек на триаже: вешает `owner-agreed`, снимает `proposed`.

### Один цикл флота

Обёртка `claude -p "/work-cycle"` → скилл `work-cycle` (оркестратор): `[CYCLE] start` →
`git fetch origin` → читает `steps/select.md` → диспатчит ровно один
`steps/{implement|review|address}.md` → `[CYCLE] done` / `[CYCLE] end #N` → процесс умирает.

`select.md` (без субагентов, только MCP): `[SELECT] scanning` → KILL-SWITCH
`mcp__github__list_issues` (`fleet-stop`) → каскад: **CHANGES** (`list_pull_requests` +
`get_pull_request`; `needs-work`, не ты последний пушивший; round≥3 → `update_issue` `needs-human`) ·
**REVIEW** (claim: `update_issue` assignees=[you] → пауза 1–3 с → `get_pull_request` перечитать →
проигрыш = yield) · **NEW ISSUE** (`list_issues` `task,owner-agreed`; блокеры `Depends on #X` через
`get_issue`; claim тот же) → `[PICKED] type=… #N` либо `WORK_QUEUE_EMPTY`.

`implement.md`: `get_issue` (непроверяемо → `needs-human`) → `git worktree add … -b feat/N-<slug>
origin/main` → план (родитель) → **coder** (Sonnet; `go-backend-conventions` + `new-service` /
`new-resource` / `frontend-conventions`): код + тесты, `make mocks`, `make test`/`cover`/`lint`,
коммит в worktree → SELF-REVIEW `git diff origin/main...HEAD` → **code-reviewer** (`review-pr`,
`go-backend-conventions`) + **security-reviewer**; фиксы → снова **coder** → родитель `git push` +
`mcp__github__create_pull_request` (`Closes #N`). Не мержит.

`review.md`: `git fetch` + `get_pull_request`/`_files`/`_status` + `get_issue` →
`get_pull_request_comments` + `gh api graphql` (reviewThreads, выделить треды бота Copilot) →
**code-reviewer** + **security-reviewer** (apply/dismiss по каждому Copilot-комментарию) → резолв
тредов `add_issue_comment` + `gh api graphql resolveReviewThread` → вердикт: **GOOD** —
`create_pull_request_review` APPROVE + `gh pr merge --auto --squash`; **BAD** — REQUEST_CHANGES +
`update_issue` (бамп `round-*`, `needs-work`/`needs-human`) + снять себя из assignees.

`address.md`: `get_pull_request`/`_files`/`_comments` → `git worktree add` + `reset --hard
origin/<branch>` (+ rebase) → **coder** (Sonnet): правки строго по комментариям → гейты зелёные →
резолв комментариев + `git push` → `update_issue` снять `needs-work`.

### Хуки (на каждое действие, вне цепочки)

`PreToolUse Edit|Write` → `block-github.mjs`; `PostToolUse Edit|Write` → `gofmt.mjs` +
`spellcheck.mjs`; `PostToolUse *` → `trace.mjs`.

### Свод «кто кого зовёт»

`work-cycle` → `select` → {`implement` | `review` | `address`}; `implement`/`address` → **coder**;
`implement`/`review` → **code-reviewer** + **security-reviewer**; **coder** тянет канон-скиллы;
`feeder` — отдельная ветка (человек). Read-only-ревьюеры наружу ничего не вызывают (нет Bash/MCP).


## Гейт проверки

Между PR и main — три слоя, от непробиваемого к совещательному.

Слой 1, CI (главный, детерминированный). Actions на каждый PR, физически блокируют мерж при
провале (required status checks в ruleset). Джобы: lint, typecheck, test (покрытие ≥80%), build,
frontend, spell (typos-корпус) и spell-diff (cspell англ.+рус. по изменённым строкам). Агент не
может уговорить CI — красный чек = мерж заблокирован. Это объективный
pass/fail, в отличие от ИИ-ревью. Required-чеки добавляются в ruleset после первого зелёного
прогона CI (до этого GitHub не покажет имена джобов); review.md дополнительно проверяет, что КАЖДАЯ
ожидаемая джоба = success (страховка от skipped/cancelled/устаревших чеков до настройки ruleset).

Слой 2, двойной агент-ревьюер (read-only). code-reviewer и security-reviewer с `tools: Read, Grep,
Glob` (без Bash, без MCP) — структурно не могут approve/merge/создать что-либо. Дифф им передаёт
родитель в промпте; сами они ничего не забирают. code-reviewer несёт `skills: [review-pr,
go-backend-conventions]`. Зовутся и в review.md, и в self-review implement.md. Вердикт выносит
родитель, не сабагент.

Слой 3, Copilot (совещательный, обязателен к резолву). Copilot комментит PR, но approve не ставит
и не required-reviewer (иначе auto-merge завис бы). Каждый его комментарий обязателен к рассмотрению
(apply/dismiss) и к резолву (иначе require-conversation-resolution заблокирует мерж), но не
обязателен к применению — один его комментарий недостаточен для провала PR без реального дефекта.

Что должно сойтись для мержа:
- зелёный CI (все required checks);
- approve от ревьюера другого аккаунта (require 1 approval; require approval of most recent push не
  даёт апрувить свой же последний push);
- все ревью-треды (включая Copilot) resolved;
- ветка не отстаёт, force-push заблокирован, squash.

Мерж не сразу: ревьюер ставит approve + auto-merge, GitHub мержит сам по зелёному CI.

Require review from Code Owners выключено — поэтому аппрув агента-ревьюера (аккаунт B) засчитывается
для PR автора (аккаунт A), и автономный мерж работает. Размен: строка `/.github/` в CODEOWNERS
(реально `@GitOleksandrBokov @andrey-morgun`) перестаёт быть барьером; `.github/` держат хук +
deny-правило.


## Жизненный цикл контекста

У флота нет общей памяти и долгоживущих сессий. Контекст очищается, пересоздаётся, передаётся и
сменяется строго определённым образом.

Очистка. `claude -p` (флаг `-p` = headless) это новый процесс ОС: рождение = пустое окно, смерть =
полная очистка. Команда `/clear` не нужна — ничто из памяти агента не переживает завершение цикла.

Пересоздание. Каждый цикл пересобирает контекст с нуля из двух источников: файлы скиллов на диске
(work-cycle → select → нужный шаг, плюс канон — воркер каждый цикл заново читает свои инструкции) и
состояние в GitHub (лейблы, assignee, открытые PR/issue, комментарии). Следствие: цикл идемпотентен
и устойчив к падению — если процесс умер на середине, истина в GitHub, следующий цикл перечитает и
продолжит.

Передача — два канала. Канал А, GitHub (между циклами и машинами): когда воркер A открыл PR, а
воркер B берёт его на ревью — это разные процессы с нулевым общим контекстом, передача целиком
через PR + дифф + issue. Когда PR забракован, третий воркер берёт его на доработку и читает
комментарии ревьюера как ТЗ — поэтому комментарии должны быть самодостаточными и построчными, их
прочитает свежий агент без общего контекста. Канал Б, промпт в сабагент (внутри цикла): родитель
сам забирает дифф через MCP и кладёт его в промпт сабагенту вместе с критериями.

Изоляция сабагентов. Родитель-воркер делегирует двум типам сабагентов в отдельных окнах: `coder`
пишет код (implement/address), `code-reviewer` и `security-reviewer` ревьюят дифф. Это даёт свежесть
(ревьюер не предвзят к коду, кодер не тащит контекст прошлой задачи) и чистоту родителя: мусор от
чтения файлов оседает в окне сабагента и не загрязняет родителя — ему возвращается только результат
(сводка кодера / вердикт ревьюера). В логе действия сабагентов видны с префиксом по типу:
`{coder}`, `{code-reviewer}`, `{security-reviewer}`.

Смена единиц. Цикл делает ровно одну единицу и завершается — он не берёт вторую в том же процессе.
Смена на другую единицу — всегда новый процесс (новый цикл), снова пустое окно. Зачем так, а не
один долгий процесс: нет накопления контекста и дрейфа между задачами, предсказуемая стоимость
цикла, чистый рестарт после сбоя.

Где живёт долговременное состояние: в GitHub (лейблы, assignee, PR, комментарии, `round-*`).


## Движение карточек по борде

Статусы двигает автоматика по событиям, не воркеры (степы карточки не трогают):
- новый issue → Todo (встроенный Project-workflow Auto-add).
- issue assigned + `owner-agreed` → In Progress (`project-in-progress.yml`).
- PR привязан к issue (`Closes #N`) → In Review (встроенный).
- changes requested → In Progress (встроенный).
- merged/closed → Done (встроенный).
- reopened → Todo (встроенный).

`project-in-progress.yml` требует секрет `PROJECTS_TOKEN` (PAT с org-доступом к Projects;
`GITHUB_TOKEN` не подойдёт). Без него автоматика борда молча падает на каждом claim, а воркеры
этого не замечают. Done флот сам не ставит — issue закрывается мержем. Борд — вспомогательная
вьюха, а не гейт: если Action упадёт, цикл флота это не сломает.


## Предохранители

- Раунды: счётчик `round-N` в лейбле PR; на ≥3 — `needs-human`, флот PR больше не трогает.
- Фикс дедлока ре-ревью: review.md в ветке BAD снимает себя из assignee, PR возвращается в пул.
- Deadlock зависимостей: все задачи заблокированы и нет другой работы — `needs-human`, выход.
- Непроверяемые критерии: implement.md шаг 1 — `needs-human`, не гадать.
- Claim против гонок: assign, задержка 1–3 с, перечитать, при проигрыше уступить.
- Kill-switch: issue с `fleet-stop` — воркеры выходят на следующем заходе.
- Claim эксклюзивен: продолжить только если assignees == ровно [ты] (защита от двойного захвата).
- Claim — лиза: протухший `[CLAIM]` (>30 мин без прогресса) переподбирается → краш не сиротит задачу.
- Частичный сбой PR: push прошёл, `create_pull_request` упал → снять assignee + `CYCLE_ERROR`; issue
  снова берётся (нет вечного залипания с orphan-веткой).
- Self-review gate: неустранимый блокер в implement → `needs-human` без открытия PR.
- Coverage: `coder` возвращает реальный %, воркер проверяет ≥80; review.md требует success по каждой CI-джобе.
- CYCLE_ERROR vs WORK_QUEUE_EMPTY: цикл различает «упал» и «очередь пуста»; обёртка громко стопает на ошибке.
- Identity: `fleet-preflight` сверяет gh/git == MCP и различие трёх аккаунтов до старта петли.


## Наблюдаемость

Механический лог — хук `trace.mjs` (matcher `*`) пишет строку на каждое действие в `$FLEET_LOG`
(фолбэк `/tmp/fleet.log`, если переменная не задана): `HH:MM:SS [машина] {agent_type?} <tool_name>
<detail>`. Действия сабагентов с префиксом по типу (`{coder}`, `{code-reviewer}`,
`{security-reviewer}`), GitHub-операции как `mcp__github__...`. Живой просмотр: `tail -f
~/fleet-logs/fleet.log`.

Смысловые маркеры — в выводе степов: `[CYCLE]`, `[SELECT]`, `[PICKED]`, `[IMPLEMENT]`, `[PR]`,
`[REVIEW]`, `[COPILOT]`, `[ADDRESS]`. Реальное время даёт хук, смысл — маркеры; stream-json/jq не
нужны.


## Запуск

По запросу на каждой из трёх машин под своим GitHub-аккаунтом. Петля дренирует очередь, пока
select.md не напечатает `WORK_QUEUE_EMPTY`. `FLEET_AGENT` — метка машины в логе, задайте разную на
каждой (`ubuntu` / `wsl` / `winbash` или `$HOSTNAME`). Формат вывода — текстовый: финальный текст
цикла (маркеры + recap) тиится в лог, а реальное время даёт хук `trace.mjs` (см. «Получение логов»).
`--output-format stream-json`/`jq` не нужны — хук уже пишет те же действия простым текстом. Перед
петлёй — один раз на каждой машине: `claude -p "/fleet-preflight"` (Git Bash: с `MSYS2_ARG_CONV_EXCL='*'`);
при `PREFLIGHT_FAIL` петлю не стартовать.

Ubuntu и Windows-WSL — нативный Linux, команда одна (на WSL claude и node ставятся ВНУТРИ WSL, не
через Windows-интероп; иначе путь лога и node разъедутся между Win и Linux). На WSL-машине поставьте
`FLEET_AGENT=wsl`:

    mkdir -p ~/fleet-logs && while out=$(FLEET_AGENT=ubuntu FLEET_LOG=~/fleet-logs/fleet.log claude -p "/work-cycle" --permission-mode auto --model opus --settings '{"ultracode": true}'); do printf '[%s] ⟵ %s\n' "$(date +%H:%M:%S)" "$out" | tee -a ~/fleet-logs/fleet.log; echo "$out" | grep -q "CYCLE_ERROR" && { printf '[%s] ⚠ CYCLE_ERROR — остановка машины\n' "$(date +%H:%M:%S)" | tee -a ~/fleet-logs/fleet.log; break; }; echo "$out" | grep -q "WORK_QUEUE_EMPTY" && break; done

Windows (Git Bash / MINGW64) — два отличия от Linux (проверено эмпирически на этой машине):
- POSIX-конверсия путей калечит аргумент: `claude -p "/work-cycle"` уходит как
  `C:/Program Files/Git/work-cycle`. Лечит `MSYS2_ARG_CONV_EXCL='*'` (тогда остаётся `/work-cycle`).
  НЕ комбинируйте с `//work-cycle` — вместе дают двойной слеш `//work-cycle`.
- `hostname -s` под Git Bash падает (это нативный `hostname.exe`) — `FLEET_AGENT` задавайте явно или
  через `$HOSTNAME`. А `FLEET_LOG=~/...` работает: MSYS конвертирует значение в `C:/Users/...`, node
  пишет туда, `tail` читает обратно через `~`.

    mkdir -p ~/fleet-logs && while out=$(MSYS2_ARG_CONV_EXCL='*' FLEET_AGENT=winbash FLEET_LOG=~/fleet-logs/fleet.log claude -p "/work-cycle" --permission-mode auto --model opus --settings '{"ultracode": true}'); do printf '[%s] ⟵ %s\n' "$(date +%H:%M:%S)" "$out" | tee -a ~/fleet-logs/fleet.log; echo "$out" | grep -q "CYCLE_ERROR" && { printf '[%s] ⚠ CYCLE_ERROR — остановка машины\n' "$(date +%H:%M:%S)" | tee -a ~/fleet-logs/fleet.log; break; }; echo "$out" | grep -q "WORK_QUEUE_EMPTY" && break; done

Получение логов (текст; `tail`/`grep`, одинаково на всех трёх):

    tail -f ~/fleet-logs/fleet.log                                                              # live: действия + recap циклов
    grep -F '[winbash]' ~/fleet-logs/fleet.log                                                  # по машине
    grep -nE '\[(CYCLE|SELECT|PICKED|IMPLEMENT|PR|REVIEW|COPILOT|ADDRESS)\]' ~/fleet-logs/fleet.log   # только вехи
    grep -F '{coder}' ~/fleet-logs/fleet.log                                                    # действия одного сабагента

`--model opus` + ultracode — самый дорогой режим (для дешевле — `--effort xhigh` вместо
`--settings`). Порядок: не запускать до фазы C (CI настоящий + required checks в ruleset). Конвейер:
`/feeder docs/` → ручной триаж → `fleet-preflight` (раз на машину) → обёртка. Стоп — issue с лейблом `fleet-stop`.


## Что держать под контролем

- `PROJECTS_TOKEN` обязателен, иначе движение карточек молча мертво. Токены истекают.
- Три разных аккаунта: на каждой машине `gh auth`/git и токен MCP = ОДИН аккаунт этой машины и
  РАЗНЫЙ между машинами, иначе merge-gate «approval не равно author» ломается. Сверяется
  `fleet-preflight` (FAIL при gh≠MCP или совпадении логинов машин).
- assignee на PR через MCP — проверить на живом PR, что `update_issue` с `assignees` реально
  назначает на PR (claim ревью завязан на это).
- Точный login бота Copilot подсмотреть на реальном PR (нужен review.md).
- `typos` в PATH на каждой машине, иначе локальный spell-хук молча отключается; гейтом остаётся CI.
- Качество бэклога = качество спеки: размытые критерии уходят в `needs-human`.
- `frontend-conventions` — стартовый канон, уточнить под реальную структуру `frontend/`.