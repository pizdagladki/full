# Автономный флот разработки — описание флоу

Как работает флот Claude Code на репозитории `pizdagladki/full`: компоненты, путь задачи от
спецификации до мержа, гейт проверки и жизненный цикл контекста воркеров.


## Коротко

Один агент на одном GitHub-аккаунте по запросу запускает петлю `claude -p "/work-cycle"`.
Один прогон петли = один цикл с чистым контекстом: воркер берёт одну единицу работы из очереди
GitHub, выполняет, печатает результат, завершает процесс. Состояние живёт в GitHub (лейблы,
assignee, PR, комментарии), а не в памяти агента. Очередь наполняет человек: `/feeder` создаёт
задачи, человек одобряет их лейблом `owner-agreed`.

Раньше планировались два-три асинхронных агента на разных аккаунтах, ревьюившие PR-ы друг друга
(кросс-аккаунт). Одной подписки на это не хватает, поэтому теперь **один агент ревьюит сам себя**.
Первое ревью идёт ПРЯМО В implement-цикле (без отдельного холодного цикла), но ВЕРДИКТ выносит не
сам оркестратор (он писал код и предвзят за-merge-ить), а **свежий сабагент `adjudicator`** с пустым
окном — это даёт контекстную декорреляцию без нового процесса. Ре-ревью после правок, legacy-PR и
поздний Copilot обслуживает отдельный свежий цикл review.md (там родитель сам новый `claude -p`).
Объективность держится не на втором аккаунте, а на трёх вещах: детерминированный CI, механический гейт
«каждый критерий ↔ именованный падающий тест», и контекстная декорреляция свежего адъюдикатора/процесса.
Честно: это **не** объективность кросс-аккаунта (та же семья весов ревьюит свой же код — общие слепые
зоны остаются); несущие гейты — CI и criterion→test, ИИ-панель совещательна (см. §Гейт).


## Принципы

Состояние живёт в GitHub, не в памяти. Любое решение, которое должно пережить цикл, пишется в
GitHub: лейбл (включая `reviewed-armed` — «self-review пройден, PR армирован на мерж»), assignee,
комментарий ревью, счётчик раундов. GitHub — шина состояния между циклами.

Каждый цикл — чистый контекст. `claude -p "/work-cycle"` это новый процесс с пустым окном. Делает
одну единицу работы и умирает. Следующий цикл — снова пустое окно. Это не только про дешёвый
рестарт: **чистый контекст — это и есть механизм объективности self-review**. Цикл, который ревьюит
PR, — отдельный процесс без памяти о написании этого кода, поэтому реконструирует всё из GitHub и
судит дифф как чужой (см. §Жизненный цикл, §Гейт).


## Компоненты

`.claude/` (в гите, общая на команду):
- `settings.json` — permissions (allow/deny) и регистрация хуков.
- `agents/coder.md` — исполнитель (Sonnet): пишет код и тесты в worktree (на каждый критерий —
  именованный падающий тест), гоняет make-гейты, коммитит локально; GitHub/PR/борд не трогает.
- `agents/code-reviewer.md` — read-only ревьюер (Opus, `tools: Read`): объединённый проход
  корректность + соответствие критериям + канон + baseline-security; калиброванная адверсариальная
  рамка (флагать только с конкретной строкой И падающим входом). `agents/security-reviewer.md` —
  глубокий security-проход (Opus, `tools: Read`), зовётся ТОЛЬКО когда дифф трогает чувствительный
  путь (`auth|oauth|session|token|payment|stripe|storage|secret`). `agents/criteria-auditor.md`
  (Opus, `tools: Read, Grep, Glob`) — несущий объективный гейт: маппинг критерий→именованный падающий
  тест (`UNTESTED` = блокер без права отмены) + аудит критериев против исходной спеки.
- `agents/adjudicator.md` (Opus, `tools: Read`) — выносит ФИНАЛЬНЫЙ вердикт (apply/dismiss + GOOD/BAD)
  по находкам панели из СВЕЖЕГО окна. Используется И в implement (родитель писал код → предвзят), И в
  review.md — родитель-оркестратор бежит на **Sonnet**, поэтому суждение нельзя оставлять на нём. Так
  весь вердикт всегда на Opus, а sonnet-оркестратор только исполняет решение.
- `hooks/gofmt.mjs` — gofmt на .go после правки.
- `hooks/block-github.mjs` — блокирует запись в `.github/`.
- `hooks/trace.mjs` — пишет строку в лог на каждое действие.
- `skills/work-cycle/` — оркестратор: `SKILL.md` + `steps/{select,implement,review,address,unblock}.md`.
- `skills/fleet-preflight/` — разовый стартовый чек: identity (gh/git == MCP), доступность MCP,
  single-account гейт-пререквизиты (ruleset требует CI, но НЕ аппрува). Запускать перед петлёй.
- `skills/feeder/` — спецификация в backlog задач.
- `skills/go-backend-conventions/`, `skills/frontend-conventions/`, `skills/review-pr/` — канон (знание).
- `skills/new-service/`, `skills/new-resource/` — скаффолдинг.

`.github/` (зона человека, флот сюда не пишет):
- `workflows/ci.yml` — джобы lint / typecheck / test / build / frontend.
- `workflows/project-in-progress.yml` — двигает карточку в In Progress по claim.
- `CODEOWNERS`, `ISSUE_TEMPLATE/task.md`, `pull_request_template.md`.

Защита `.github/`: хук `block-github.mjs` + deny-правило в `settings.json` + CODEOWNERS.

Транспорт (чем что делается):
- issue/PR (get/create/update, комментарии, лейблы, assignee, ревью) — GitHub MCP (`mcp__github__*`).
- движение карточек по борде — автоматика (GitHub Actions + встроенные Project-воркфлоу); шаги
  work-cycle борд не трогают. Исключение: feeder кладёт новый issue на борд через
  `gh project item-add` (Bash; `gh project:*` разрешён в `settings.json`).
- включить auto-merge — `gh pr merge <N> --auto --squash` (Bash).
- подтянуть отставшую ветку под main — `gh pr update-branch <N>` (Bash; мержит main в ветку PR, без force-push) либо `git merge origin/main` в worktree. Rebase+force-push запрещён (`settings.json` + branch protection) — ребейзить запушенную ветку нельзя.
- резолв Copilot-тредов — `gh api graphql ... resolveReviewThread` (Bash).
- локальный код (git worktree/merge/push, make, gofmt) — Bash.

MCP merge-инструмент не используется нигде — он мержит немедленно, в обход «ждать зелёного CI».


## Лейблы

Десять лейблов — это всё координационное состояние системы. Воркеры читают их каждый цикл.

- `task` — задача для флота (вешает feeder, всегда с `proposed`).
- `owner-agreed` — задача допущена в очередь (вешает человек на триаже).
- `proposed` — создано feeder, человек ещё не смотрел.
- `needs-work` — по PR запрошены правки, доработать (вешает review.md / CI-RECOVERY в select.md).
- `reviewed-armed` — self-review пройден, auto-merge армирован, PR едет на мерж по зелёному CI
  (вешает review.md; снимает любой новый push — address.md и conflict-путь unblock.md). Это «ревью
  сделано»-сигнал select.md ВМЕСТО мёртвого `reviewDecision` (само-аппрува на своём PR GitHub не даёт).
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
4. Агент (`/work-cycle`) дренирует очередь: берёт задачи с `task + owner-agreed`,
   у которых все блокеры закрыты.
5. Цикл реализации: implement → in-cycle self-review (панель + свежий `adjudicator`-сабагент выносит вердикт) → PR открывается уже армированным (`reviewed-armed`) → (при правках address → ре-ревью отдельным циклом review.md) → (отстала от main / конфликт / CI покраснел после армирования) unblock / CI-RECOVERY → мерж по зелёному CI. Нет approve-события — один аккаунт не может за-аппрувить свой PR; гейт = зелёный CI + резолв тредов + up-to-date ветка.
6. Мерж закрывает issue (`Closes #N`) и разблокирует зависимые задачи — флот берёт их следующими.

Про feeder: он идемпотентен. В тело каждой задачи прячется отпечаток `<!-- fdr-<area>-<short-slug> -->`,
и перед созданием feeder ищет его среди всех issues (любого статуса) — повторный прогон не
воссоздаёт ни одобренную задачу (с неё снят `proposed`), ни закрытую. **Обновление спеки:** при
overlap'е feeder не дропает вслепую, а диффит новые acceptance-критерии с критериями существующей
issue — совпали → skip; разошлись + issue закрыта/смержена (код уже на `main`) → создаёт
`…-reconcile`-задачу (правка ПОВЕРХ существующего кода, не баг-репорт и не переписывание с нуля);
разошлись + issue ещё открыта → флаг человеку на re-triage. Feeder диффит спеку с issue, не с кодом —
ловит дрейф там, где на функционал есть задача. Ручные prerequisite (OAuth, Stripe, инфра) feeder
задачами не делает — только помечает `Manual prerequisite (human): …` в Context.

Триаж — ручной гейт. Без `owner-agreed` задача невидима для флота.


## Один цикл /work-cycle

Оркестратор `work-cycle/SKILL.md` читает `steps/select.md`, выбирает одну единицу работы, запускает
один из `steps/{implement,review,address,unblock}.md` по типу, печатает маркеры и завершается.
`disable-model-invocation: true` — запускается только обёрткой.

### select.md — выбор одной единицы

Шаг 0, KILL-SWITCH: если есть открытая issue с `fleet-stop` — печать `WORK_QUEUE_EMPTY`, выход.

SINGLE-AGENT: один аккаунт ревьюит и мержит СВОИ PR — исключения «не автор / не последний пушивший»
убраны. `reviewDecision` мёртв (без approve-события он навсегда ≠ `APPROVED`), сигнал «ревью сделано»
— лейбл `reviewed-armed`. Дальше приоритетный каскад (закончить важнее, чем начать), первый подходящий:

1. CHANGES — PR с `needs-work`. Если `round-N ≥ 3` — поставить `needs-human`, снять `needs-work`,
   пропустить. Иначе тип address.
2. CI-RECOVERY — `reviewed-armed` PR (армирован, исключён из REVIEW), у которого на текущем head.sha
   required-джоба = `failure`/`cancelled`/`timed_out` (`gh pr checks`). Auto-merge на красном чеке не
   сработает никогда — без этой ветки PR вечный зомби. Снять `reviewed-armed`, повесить `needs-work` +
   коммент с упавшими джобами, тип address. (Не-отревьюенный PR без `reviewed-armed` с красным CI идёт
   в REVIEW→BAD, не сюда.)
3. UNBLOCK — открытый PR БЕЗ `needs-work`, который GitHub сам не вмержит (`gh pr view --json
   mergeStateStatus,mergeable,autoMergeRequest`): `CONFLICTING` (конфликт с main); ИЛИ `BEHIND` (отстал
   от main); ИЛИ `reviewed-armed` + мёржабелен, но auto-merge не армнут. Свой PR — это механический
   ре-синк, не ревью. Страховка от зависания армированных PR за main (см. §Гейт).
4. REVIEW — открытый свой PR без `needs-work`, которому нужно решение self-review: либо нет
   `reviewed-armed` (не ревьюен, или новый push снял лейбл), либо `reviewed-armed`, но есть
   нерезолвленные треды (поздний Copilot после армирования — review.md резолвит их threads-only, без
   панели). `reviewed-armed` со всеми резолвленными тредами исключён (тихо едет на мерж по зелёному CI).
   claimable. Иначе тип review.
5. NEW ISSUE — open issue с обоими лейблами `task` и `owner-agreed`, claimable, все блокеры
   `Depends on #X` закрыты. Без `owner-agreed` issue игнорируется.

Claim — эксклюзивная лиза, не лок: назначить себя + штамп-коммент `🤖 [CLAIM] <машина> <ts>`, задержка
1–3 с, перечитать и продолжить ТОЛЬКО если assignees == ровно [ты], иначе уступить. С одним агентом
гонок нет — claim остаётся как переподбор осиротевшей крашем работы: протухший `[CLAIM]` (>30 мин без
прогресса) или своя задача от упавшего цикла перезахватывается. Прогресс меряется ОТНОСИТЕЛЬНО штампа
claim (новый push / смена лейбла ПОСЛЕ него).

Deadlock breaker: если `task+owner-agreed` issues есть, но все заблокированы (или цикл A↔B) и нет
другой работы — поставить `needs-human` на старейшую, печать `WORK_QUEUE_EMPTY`, выход.

Нет категории `no-eligible-reviewer` — один аккаунт ревьюит свою же работу. Устойчивое состояние «все
открытые PR `reviewed-armed` с резолвленными тредами, ждут зелёного CI» — это тоже `WORK_QUEUE_EMPTY`
(ничего не actionable; следующая итерация петли перепроверит, когда CI устаканится).

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
5. `coder` гоняет `make test`, `make cover` (≥80%), `make lint` до зелёного. На каждый критерий —
   именованный падающий тест. Он возвращает % покрытия (строка `total: … %`) и маппинг критерий→тест;
   воркер проверяет ≥80% сам. Не вышло — вернуть пробел.
6. RE-SYNC FIRST: `git merge origin/main` в ветку (конфликты решает `coder`) — чтобы self-review
   ревьюил ФИНАЛЬНЫЙ дифф (включая результат мержа), а не до-мерж-состояние.
7. IN-CYCLE SELF-REVIEW (объективный гейт, тёплый — без отдельного холодного цикла; но ВЕРДИКТ выносит
   свежий сабагент, не сам оркестратор). Дифф `git diff origin/main...HEAD` → ПАНЕЛЬ (свежие окна):
   `code-reviewer` + `criteria-auditor` всегда, `security-reviewer` только на sensitive-пути → ВЕРДИКТ
   делегируется свежему `adjudicator`-сабагенту (ему дают дифф + критерии + находки панели, НЕ план
   родителя; `UNTESTED` = механический BAD). BAD → `coder` фиксит → перепрогон панели+адъюдикатора; не
   устранилось за один ре-делегейт → `needs-human`, выход БЕЗ PR. GOOD → дальше.
8. Запушить ветку, открыть PR (`Closes #N`, вывод тестов, заполненный PR-шаблон), запостить
   dismissed-находки одним аудит-комментом. Self-review уже пройден → **ARM** `gh pr merge --auto
   --squash` (проверить) + лейбл `reviewed-armed`. Если push прошёл, а `create_pull_request` упал —
   снять себя из assignee issue, `CYCLE_ERROR pr-create-failed`, выход.
9. Выход. PR уже армирован и едет на мерж по зелёному CI. Поздний Copilot-тред на нём подберёт
   select.md REVIEW (`reviewed-armed` + нерезолвленные треды) → review.md threads-only; красный CI
   после армирования — CI-RECOVERY.

### review.md — SELF-REVIEW своего PR: arm или needs-work

Первое ревью нового PR делает implement in-cycle (через `adjudicator`), поэтому review.md обслуживает
ОСТАЛЬНОЕ: ре-ревью после `address`, legacy In-Review PR из кросс-аккаунт-перехода, и поздние
Copilot-треды на армированном PR. Свежий процесс без памяти о написании кода реконструирует всё из
GitHub и судит дифф как чужой. Вердикт ЗДЕСЬ тоже выносит opus-`adjudicator`-сабагент (а НЕ родитель):
родитель-оркестратор бежит на Sonnet, поэтому суждение держим на Opus, а он лишь гоняет панель,
дёргает adjudicator и исполняет решение. Несущая объективность — в CI и маппинге критерий→тест;
ИИ-панель совещательна (см. §Гейт).

1. `git fetch`, сверить актуальный head. Забрать дифф + критерии + путь исходной спеки через MCP.
   THREADS-ONLY SHORT-CIRCUIT: если PR уже `reviewed-armed` и новых коммитов нет (прилетели только
   нерезолвленные треды — поздний Copilot) — НЕ гонять панель, сразу к шагам 2+4 (резолв) и переармить.
2. CI по-джобно (`get_check_runs`): любая required-джоба `failure`/`cancelled`/`timed_out`/устаревшая
   на head.sha = CI-RED (жёсткий BAD); все success = CI-GREEN; часть бежит и нет провалов = CI-PENDING
   (армировать поверх можно — auto-merge ждёт зелёного; поздний красный ловит CI-RECOVERY).
3. Copilot: через GraphQL получить ревью-треды, отделить авторства бота. Не пришло — не блокироваться.
4. Делегировать калиброванной панели (родитель дёшево прикладывает соседние файлы — не давать трём
   агентам краулить): ВСЕГДА `code-reviewer` (объединённый correctness+критерии+канон+baseline-security,
   diff-in-prompt) + `criteria-auditor` (Read; критерий→именованный тест и аудит критериев-против-спеки);
   `security-reviewer` — ТОЛЬКО если дифф трогает `auth|oauth|session|token|payment|stripe|storage|secret`.
   Ре-ревью (был `needs-work`): передавать сырые факты тредов, НЕ прозу прошлого ревьюера (анкорит).
5. Адъюдикация (родитель context-независим, но с жёсткими правилами против throughput-bias):
   `UNTESTED` от аудитора = МЕХАНИЧЕСКИЙ блокер без права отмены; спека-vs-критерии gap (correctness/
   security) → `needs-human`; открытые находки `apply` только при конкретной строке И падающем входе,
   иначе `dismiss` с записью ДОСЛОВНО в коммент PR (аудируемо). Резолв всех Copilot-тредов в обеих ветках.
6. Вердикт:
    - GOOD = нет применённых блокеров, нет `UNTESTED`, нет correctness/security-gap, CI не CI-RED.
      Сначала ГЕЙТ МЁРЖАБЕЛЬНОСТИ: `gh pr view --json mergeStateStatus,mergeable` — `BEHIND`/`CONFLICTING`
      → НЕ армить в этот цикл (снять assignee, `good-but-unmergeable -> unblock`, выход; ветку приведёт
      UNBLOCK, свежий цикл переармит актуальный head). Иначе: `gh pr merge --auto --squash` (НЕТ
      approve-события — само-аппрув GitHub запрещён, а ослабленный ruleset аппрува не требует), ПРОВЕРИТЬ
      что армнулся (`autoMergeRequest` ≠ null, иначе `CYCLE_ERROR`), повесить `reviewed-armed` (сигнал
      «ревью сделано»), снять assignee. Мержится по зелёному CI.
    - BAD = применённый блокер / `UNTESTED` / correctness-spec-gap / CI-RED. Если был `reviewed-armed` —
      снять. Самодостаточные комментарии (применённые Copilot-находки своими словами; для CI-RED —
      упавшие джобы), лейбл `needs-work`, бамп раунда (нет `round-*` → `round-1`; иначе +1), снять
      assignee (PR → CHANGES → address). REQUEST_CHANGES-событие НЕ слать — на своём PR оно
      бессмысленно; `needs-work` — это маршрутный сигнал. При раунде ≥3 — `needs-human`.

### address.md — доработать PR

Как и в implement.md, правки пишет субагент `coder`, не сам воркер.

1. Забрать PR + все комментарии ревью через MCP. Это единственный контекст. Отдельно гоняться за
   Copilot-тредами не нужно — ревьюер их уже адъюдицировал и зарезолвил.
2. Сначала синк: `git fetch`; worktree на ветке PR, сброшенный к её последнему удалённому head
   (`git worktree add ../<branch> <branch>` + `git reset --hard origin/<branch>`). Если отстаёт от
   `origin/main` — **merge** `origin/main` в ветку (не rebase: rebase переписывает историю → push
   требует `--force`, а он запрещён `settings.json` и branch protection; merge оставляет push
   fast-forward), конфликты решает `coder`.
3. Делегировать правки субагенту `coder`: дать ему worktree, зону и комментарии ревьюера. Он правит
   строго по комментариям (без расширения scope) и коммитит; код руками не править.
4. `coder` держит `make test`, `make cover` (≥80%), `make lint` зелёными — воркер лишь подтверждает.
5. Резолв комментариев, push.
6. Снять `needs-work` И, если есть, `reviewed-armed` (новый дифф не отревьюен — select.md ключит
   ре-ревью на отсутствии `reviewed-armed`). PR возвращается на self-review. Раунды/борд не трогать.
7. Выход.

### unblock.md — вернуть застрявший PR в мёржабельное состояние

Механический ре-синк `reviewed-armed`/конфликтного PR, который GitHub сам не вмержит. НЕ ревью —
аппрува здесь нет (свой PR). Ты уже заклеймил PR.

1. Свежее состояние: `gh pr view <N> --json mergeStateStatus,mergeable,autoMergeRequest,...`.
   Если PR уже `CLEAN`/`UNSTABLE` с армнутым auto-merge (едет сам), draft или закрыт — снять claim,
   `already-progressing`, выход. `UNKNOWN` — перечитать раз, иначе выйти (другой цикл повторит).
2. `BEHIND` без конфликта: `gh pr update-branch <N>` (мерж main в ветку, без force-push; `--rebase`
   НЕ использовать). Чистый мерж main — это уже отревьюенный код, поэтому `reviewed-armed` ОСТАВЛЯЕМ:
   auto-merge переживает апдейт и срабатывает по зелёному, а семантический слом ловит CI (`test`
   перебежит на новом head; красный → CI-RECOVERY). Лишний прогон через REVIEW тут не нужен — это churn.
3. `CONFLICTING`/`DIRTY`: worktree на head ветки, `git merge origin/main`, конфликт решает `coder`
   (зелёные гейты), `git push` (fast-forward, без force). Не вышло механически — `needs-human` +
   коммент, выход. Резолв конфликта = НОВЫЙ непроверенный код → СНЯТЬ `reviewed-armed`: свежий REVIEW
   переревьюит и переармит результат.
4. Догарантировать armed auto-merge ТОЛЬКО если PR остаётся `reviewed-armed` (чистый-`BEHIND`-путь):
   слетел → `gh pr merge <N> --auto --squash`, проверить. На conflict-пути лейбл снят — не переармливать
   (это сделает REVIEW). Раунды/борд не трогать.
5. Снять claim. Если снимал `reviewed-armed` (конфликт) — свежий REVIEW переревьюит и переармит head.
   Выход.


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
`steps/{implement|review|address|unblock}.md` → `[CYCLE] done` / `[CYCLE] end #N` → процесс умирает.

`select.md` (без субагентов, MCP + `gh pr view`): `[SELECT] scanning` → KILL-SWITCH
`mcp__github__list_issues` (`fleet-stop`) → каскад (свои PR разрешены; сигнал «ревью сделано» —
лейбл `reviewed-armed`, не `reviewDecision`): **CHANGES** (`needs-work`; round≥3 → `needs-human`) ·
**CI-RECOVERY** (`reviewed-armed` + required-джоба красная на head → снять `reviewed-armed` +
`needs-work` → address) · **UNBLOCK** (без `needs-work`; `gh pr view --json
mergeStateStatus,mergeable,autoMergeRequest`; match: `CONFLICTING` ИЛИ `BEHIND` ИЛИ
`reviewed-armed`+мёржабелен+auto-merge не армнут) · **REVIEW** (без `needs-work`; нет `reviewed-armed`
ИЛИ есть нерезолвленные треды; claim) · **NEW ISSUE** (`list_issues` `task,owner-agreed`; блокеры
`Depends on #X`) → `[PICKED] type=… #N` либо `WORK_QUEUE_EMPTY`.

`implement.md`: `get_issue` (непроверяемо → `needs-human`) → `git worktree add … -b feat/N-<slug>
origin/main` → план (родитель) → **coder** (Sonnet; `go-backend-conventions` + `new-service` /
`new-resource` / `frontend-conventions`): код + тесты (именованный падающий тест на критерий),
`make mocks`, `make test`/`cover`/`lint`, коммит в worktree → re-sync (`git merge origin/main`) →
IN-CYCLE self-review: панель (**code-reviewer** + **criteria-auditor** [+ **security-reviewer** на
sensitive-пути]) → **adjudicator** (свежий вердикт; `UNTESTED` = механический BAD; BAD → coder фиксит →
перепрогон; не устранилось → `needs-human`, без PR) → `git push` + `mcp__github__create_pull_request`
(`Closes #N`) → **ARM** `gh pr merge --auto --squash` + лейбл `reviewed-armed`.

`review.md` (SELF-REVIEW, свежий процесс): `git fetch` + `get_pull_request`/`_diff` + `get_issue`
(критерии + путь спеки) → CI по-джобно (`get_check_runs`) → `gh api graphql` (reviewThreads, треды
Copilot) → панель: **code-reviewer** (всегда) + **criteria-auditor** (всегда) + **security-reviewer**
(только sensitive-путь) → **adjudicator** (вердикт; родитель на Sonnet → судит Opus-adjudicator; UNTESTED =
механический блокер; dismiss → дословно в коммент) + резолв тредов → **GOOD** — гейт мёржабельности (`BEHIND`/`CONFLICTING` → снять assignee +
defer to unblock) → `gh pr merge --auto --squash` (проверить, что армнулся; БЕЗ approve-события) +
лейбл `reviewed-armed` + снять assignee; **BAD** — снять `reviewed-armed` если был + комментарии +
`update_issue` (бамп `round-*`, `needs-work`/`needs-human`) + снять assignee.

`unblock.md` (без ревью): `gh pr view --json mergeStateStatus,mergeable,autoMergeRequest`
→ `BEHIND` → `gh pr update-branch` (merge main, без force; `reviewed-armed` ОСТАЁТСЯ — едет по
зелёному) · `CONFLICTING` → `git worktree add` + `git merge origin/main` → **coder** резолвит →
`git push` (fast-forward) + СНЯТЬ `reviewed-armed` (новый код → re-review); не вышло → `needs-human`
→ догарантировать armed (только если остался `reviewed-armed`) → снять assignees.

`address.md`: `get_pull_request`/`_files`/`_comments` → `git worktree add` + `reset --hard
origin/<branch>` (отстал → `git merge origin/main`, НЕ rebase) → **coder** (Sonnet): правки строго по
комментариям → гейты зелёные → резолв комментариев + `git push` → `update_issue` снять `needs-work`.

### Хуки (на каждое действие, вне цепочки)

`PreToolUse Edit|Write` → `block-github.mjs`; `PostToolUse Edit|Write` → `gofmt.mjs`;
`PostToolUse *` → `trace.mjs`.

### Свод «кто кого зовёт»

`work-cycle` → `select` → {`implement` | `review` | `address` | `unblock`}; `implement`/`address`/`unblock` → **coder**;
`implement` → **code-reviewer** + **criteria-auditor** (+ **security-reviewer** на sensitive-пути) + **adjudicator** (вердикт);
`review` → те же панель-агенты + **adjudicator** (вердикт; родитель-оркестратор на Sonnet, судит Opus-adjudicator);
**coder** тянет канон-скиллы; `feeder` — отдельная ветка (человек). Read-only-агенты наружу ничего не вызывают (нет Bash/MCP).


## Гейт проверки

ЧЕСТНО ПРО ОБЪЕКТИВНОСТЬ. Раньше мерж требовал аппрува с ДРУГОГО аккаунта — это давало декорреляцию
по идентичности (branch protection её enforce-ил). С одним аккаунтом её нет, и вернуть её нельзя:
GitHub не даёт за-аппрувить свой PR. Что мы СОХРАНЯЕМ — декорреляцию по КОНТЕКСТУ: self-review идёт в
ОТДЕЛЬНОМ свежем процессе без памяти о написании кода. Чего не давал НИКОГДА ни кросс-аккаунт, ни
self-review — декорреляции по ВЕСАМ: и coder (Sonnet), и ревьюеры (Opus) одной семьи, общие слепые
зоны остаются. Поэтому несущую объективность нельзя вешать на ИИ-панель. Несущие — два
МОДЕЛЬ-НЕЗАВИСИМЫХ сигнала; ИИ-панель совещательна.

Слои от непробиваемого к совещательному:

Слой 1, CI (главный, детерминированный, МОДЕЛЬ-НЕЗАВИСИМЫЙ). Actions на каждый PR, физически блокируют
мерж при провале (required status checks в ruleset). Джобы: lint, typecheck, test (покрытие ≥80%),
build, frontend. Агент не может уговорить CI — красный чек = мерж заблокирован.
review.md дополнительно проверяет, что КАЖДАЯ ожидаемая джоба = success на текущем head.sha (страховка
от skipped/cancelled/устаревших), и красный CI → BAD; поздний красный после армирования ловит
CI-RECOVERY в select.md.

Слой 2, маппинг критерий→тест (механический, МОДЕЛЬ-НЕЗАВИСИМЫЙ — это и есть замена объективности
кросс-аккаунта). coder на КАЖДЫЙ acceptance-критерий пишет именованный падающий тест; implement тёплым
self-check'ом это сверяет ДО PR; `criteria-auditor` в review.md перепроверяет (`UNTESTED` = блокер без
права отмены родителем) И аудитит критерии против исходной спеки (требование из спеки, выпавшее из
критериев → `needs-human`); и в конце те же тесты бесплатно гоняются в CI-джобе `test`. «Каждый
критерий имеет падающий-при-нарушении тест» — проверяемый факт, а не суждение модели.

Слой 3, self-review-панель (совещательная, контекстно-декоррелированная, НЕ модель-независимая).
Панель read-only (`tools: Read`, без Bash/MCP — структурно не могут merge/approve): `code-reviewer`
(всегда, объединённый correctness+критерии+канон+baseline-security, калиброванно-адверсариальный:
блокер только с конкретной строкой И падающим входом — иначе false-positive churn жжёт раунды) +
`criteria-auditor` (всегда) + `security-reviewer` (только sensitive-путь). КТО выносит вердикт зависит
вердикт ВСЕГДА выносит свежий opus-сабагент **`adjudicator`** — и в **implement** (родитель писал код →
предвзят), и в **review.md** (родитель-оркестратор бежит на Sonnet, суждение нельзя оставлять на нём).
Родитель лишь гоняет панель, дёргает adjudicator и исполняет решение. В обоих
dismiss-ы пишутся дословно в коммент (аудируемо), слой-2-блокер (`UNTESTED`) не отменяем. Честно: эта
панель ловит presentation-level дефекты, но разделяет слепые зоны автора — её молчание НЕ
доказательство корректности.

Слой 4, Copilot (совещательный, обязателен к резолву). Комментит PR, но approve не ставит и не
required-reviewer. Каждый коммент обязателен к рассмотрению (apply/dismiss) и резолву (иначе
require-conversation-resolution заблокирует мерж), но не обязателен к применению.

Слой 5, человек на триаже (`owner-agreed`) — единственный не-Claude в петле. На нём — ПОЛНОТА
критериев: correlated-blindspot дефекты ловят только CI, аудит критериев-против-спеки и
человек+пост-мерж баг-репорты, а не ИИ-панель.

Что должно сойтись для мержа (single-account):
- зелёный CI (все required checks);
- НЕТ требования аппрува (ruleset ослаблен: один аккаунт не может за-аппрувить свой PR — иначе всё
  висит вечно); вместо аппрува — лейбл `reviewed-armed` от self-review и армированный auto-merge;
- все ревью-треды (включая Copilot) resolved;
- ветка не отстаёт, force-push заблокирован, squash.

Мерж не сразу: review.md армит auto-merge, GitHub мержит сам по зелёному CI.

Как ветка держится «не отстающей». GitHub auto-merge сам отставшую ветку НЕ подтягивает, merge queue
не настроен. Как только первый PR пачки вмержился, остальные `reviewed-armed` оказываются `BEHIND` —
закрывает **unblock.md** (`gh pr update-branch`, merge main, без force). Синк всегда merge, не rebase
(force-push запрещён). Чистый `BEHIND`-апдейт оставляет `reviewed-armed` (едет по зелёному, семантику
ловит CI) — без лишнего churn; только резолв реального КОНФЛИКТА снимает `reviewed-armed` и гонит на
re-review (там новый код). Полностью churn убрал бы GitHub Merge Queue, но он требует правок в
`.github/` (зона человека). Держим как апгрейд; пока хватает unblock.md.

Single-account ruleset: require-approvals и «approval of most recent push» ВЫКЛЮЧЕНЫ (иначе свой PR не
вмержить); required CI-checks и require-conversation-resolution ВКЛЮЧЕНЫ — это и есть весь биндящий
гейт. `.github/` (включая CODEOWNERS) держат хук `block-github.mjs` + deny-правило в `settings.json`.


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

Передача — два канала. Канал Б, промпт в сабагент (внутри цикла) — главный для объективности первого
ревью: родитель кладёт дифф + критерии + находки панели в промпт СВЕЖЕГО `adjudicator`-сабагента,
который и выносит вердикт. Контаминированный контекст родителя в этот промпт НЕ попадает — это и есть
декорреляция без нового процесса. Канал А, GitHub (между циклами): когда PR забракован, ещё один цикл
берёт его на доработку (`address`) и читает комментарии как ТЗ; затем review.md (свежий процесс)
ре-ревьюит — поэтому комментарии самодостаточны и построчны, их прочитает свежий агент без общего
контекста. Так же передаются legacy-PR.

Изоляция сабагентов. Родитель-воркер делегирует сабагентам в отдельных окнах: `coder` пишет код
(implement/address/unblock); панель `code-reviewer` + `criteria-auditor` (+ `security-reviewer` на
sensitive-пути) ревьюит дифф; `adjudicator` выносит вердикт (в implement И review). Это даёт свежесть (ревьюер
и адъюдикатор не предвзяты к коду, кодер не тащит контекст прошлой задачи) и чистоту родителя: мусор от
чтения файлов оседает в окне сабагента — родителю возвращается только результат. В логе действия
сабагентов видны с префиксом по типу: `{coder}`, `{code-reviewer}`, `{criteria-auditor}`,
`{security-reviewer}`, `{adjudicator}`.

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
- Сигнал «ревью сделано» — лейбл `reviewed-armed`, не мёртвый `reviewDecision` (без approve-события он
  навсегда ≠ `APPROVED`). Снимается любым новым push (address / conflict-resolve unblock) → новый дифф
  обязан пройти re-review. Без лейбла select.md зацикливался бы на ре-ревью уже-готовых PR.
- Инвариант `reviewed-armed` ⟺ armed auto-merge: КАЖДАЯ точка снятия лейбла (review BAD, CI-RECOVERY,
  conflict-resolve unblock, address) ОБЯЗАНА разармить auto-merge (`gh pr merge --disable-auto`); армит
  и вешает лейбл только review GOOD. Иначе забракованный/чинимый PR с зелёным CI вмержил бы непроверенный
  дифф ДО ре-ревью — самая опасная дыра single-account.
- CI-зомби: `reviewed-armed` PR с красным required-чеком auto-merge не вмержит никогда — ловит
  CI-RECOVERY (снять `reviewed-armed` + `needs-work` → address), иначе PR висел бы вечно невидимым.
- Фикс дедлока ре-ревью: review.md и в BAD, и в GOOD снимает себя из assignee — PR не остаётся за
  вышедшим воркером.
- Зависший мерж: `reviewed-armed` PR, отставший от main или конфликтующий, ловит UNBLOCK
  (`gh pr update-branch` / merge+coder). Claim-staleness меряет прогресс относительно штампа claim
  (новый push / смена лейбла), а не по факту старого ревью.
- Синк только merge, не rebase: force-push запрещён (`settings.json`) и заблокирован — merge держит
  push fast-forward (implement/address/unblock).
- Армирование auto-merge проверяется: review.md/unblock.md убеждаются, что `autoMergeRequest` ≠ null,
  иначе `CYCLE_ERROR` — не рапортовать успех на PR, который не вмержится.
- Несущая объективность модель-независима: детерминированный CI + механический гейт «критерий→тест»
  (`UNTESTED` от `criteria-auditor` неотменяем родителем; те же тесты гоняются в CI). ИИ-панель
  совещательна; её dismiss-ы пишутся дословно в коммент (аудируемо).
- Объективность self-review = вердикт выносит свежий opus-`adjudicator`-сабагент, а не родитель-оркестратор
  (он бежит на Sonnet ради экономии — суждение держим на Opus). Так и в implement, и в review.md. Несущие гейты
  (CI + criterion→test) от этого не зависят вовсе.
- Калибровка против churn: блокер только с конкретной строкой И падающим входом — иначе адверсариальная
  панель плодила бы false-positive раунды и сливала бы очередь в `needs-human`.
- Deadlock зависимостей: все задачи заблокированы и нет другой работы — `needs-human`, выход.
- Непроверяемые критерии: implement.md шаг 1 — `needs-human`, не гадать.
- Claim — лиза: протухший `[CLAIM]` (>30 мин без прогресса) или своя задача от упавшего цикла
  перезахватывается → краш не сиротит задачу (с одним агентом гонок нет, но переподбор остаётся).
- Kill-switch: issue с `fleet-stop` — воркер выходит на следующем заходе.
- Частичный сбой PR: push прошёл, `create_pull_request` упал → снять assignee + `CYCLE_ERROR`; issue
  снова берётся (нет вечного залипания с orphan-веткой).
- Coverage: `coder` возвращает реальный % + маппинг критерий→тест, воркер проверяет ≥80; review.md
  требует success по каждой CI-джобе.
- CYCLE_ERROR vs WORK_QUEUE_EMPTY: цикл различает «упал» и «очередь пуста»; обёртка громко стопает на ошибке.
- Identity: `fleet-preflight` сверяет gh/git == MCP и что ruleset НЕ требует аппрува (single-account
  self-merge) до старта петли.


## Наблюдаемость

Механический лог — хук `trace.mjs` (matcher `*`) пишет строку на каждое действие в `$FLEET_LOG`
(фолбэк `/tmp/fleet.log`, если переменная не задана): `HH:MM:SS [машина] {agent_type?} <tool_name>
<detail>`. Действия сабагентов с префиксом по типу (`{coder}`, `{code-reviewer}`,
`{criteria-auditor}`, `{security-reviewer}`), GitHub-операции как `mcp__github__...`. Живой просмотр:
`tail -f ~/fleet-logs/fleet.log`.

Смысловые маркеры — в выводе степов: `[CYCLE]`, `[SELECT]`, `[PICKED]`, `[IMPLEMENT]`, `[PR]`,
`[REVIEW]`, `[COPILOT]`, `[ADDRESS]`, `[UNBLOCK]`. Реальное время даёт хук, смысл — маркеры;
stream-json/jq не нужны.


## Запуск

По запросу на ОДНОЙ машине под своим GitHub-аккаунтом (single-agent: одна подписка — один агент,
который сам implement'ит, в отдельном свежем цикле self-review'ит и мержит). Петля дренирует очередь,
пока select.md не напечатает `WORK_QUEUE_EMPTY`. `FLEET_AGENT` — метка машины в логе (`$HOSTNAME` или
любая). Формат вывода — текстовый: финальный текст цикла (маркеры + recap) тиится в лог, реальное
время даёт хук `trace.mjs`. `--output-format stream-json`/`jq` не нужны. Перед петлёй — один раз:
`claude -p "/fleet-preflight"` (Git Bash: с `MSYS2_ARG_CONV_EXCL='*'`); при `PREFLIGHT_FAIL` петлю не
стартовать.

Ubuntu и Windows-WSL — нативный Linux, команда одна (на WSL claude и node ставятся ВНУТРИ WSL, не
через Windows-интероп; иначе путь лога и node разъедутся между Win и Linux). На WSL-машине поставьте
`FLEET_AGENT=wsl`:

    mkdir -p ~/fleet-logs && while out=$(FLEET_AGENT=ubuntu FLEET_LOG=~/fleet-logs/fleet.log claude -p "/work-cycle" --permission-mode auto --model sonnet --effort high); do printf '[%s] ⟵ %s\n' "$(date +%H:%M:%S)" "$out" | tee -a ~/fleet-logs/fleet.log; echo "$out" | grep -q "CYCLE_ERROR" && { printf '[%s] ⚠ CYCLE_ERROR — остановка машины\n' "$(date +%H:%M:%S)" | tee -a ~/fleet-logs/fleet.log; break; }; echo "$out" | grep -q "WORK_QUEUE_EMPTY" && break; done

Windows (Git Bash / MINGW64) — два отличия от Linux (проверено эмпирически на этой машине):
- POSIX-конверсия путей калечит аргумент: `claude -p "/work-cycle"` уходит как
  `C:/Program Files/Git/work-cycle`. Лечит `MSYS2_ARG_CONV_EXCL='*'` (тогда остаётся `/work-cycle`).
  НЕ комбинируйте с `//work-cycle` — вместе дают двойной слеш `//work-cycle`.
- `hostname -s` под Git Bash падает (это нативный `hostname.exe`) — `FLEET_AGENT` задавайте явно или
  через `$HOSTNAME`. А `FLEET_LOG=~/...` работает: MSYS конвертирует значение в `C:/Users/...`, node
  пишет туда, `tail` читает обратно через `~`.

    mkdir -p ~/fleet-logs && while out=$(MSYS2_ARG_CONV_EXCL='*' FLEET_AGENT=winbash FLEET_LOG=~/fleet-logs/fleet.log claude -p "/work-cycle" --permission-mode auto --model sonnet --effort high); do printf '[%s] ⟵ %s\n' "$(date +%H:%M:%S)" "$out" | tee -a ~/fleet-logs/fleet.log; echo "$out" | grep -q "CYCLE_ERROR" && { printf '[%s] ⚠ CYCLE_ERROR — остановка машины\n' "$(date +%H:%M:%S)" | tee -a ~/fleet-logs/fleet.log; break; }; echo "$out" | grep -q "WORK_QUEUE_EMPTY" && break; done

Получение логов (текст; `tail`/`grep`):

    tail -f ~/fleet-logs/fleet.log                                                              # live: действия + recap циклов
    grep -F '[winbash]' ~/fleet-logs/fleet.log                                                  # по машине
    grep -nE '\[(CYCLE|SELECT|PICKED|IMPLEMENT|PR|REVIEW|COPILOT|ADDRESS|UNBLOCK)\]' ~/fleet-logs/fleet.log   # только вехи
    grep -F '{coder}' ~/fleet-logs/fleet.log                                                    # действия одного сабагента

**Модель-тиринг (экономия без потери качества ревью).** Оркестратор work-cycle — это в основном клей
(select/claim/диспатч/выполнить вердикт), поэтому он бежит на **`--model sonnet`**. Всё качество-критичное
суждение — в сабагентах с прибитым `model: opus` (`code-reviewer`, `criteria-auditor`, `security-reviewer`,
`adjudicator`): их `--model` сессии не трогает. ВЕРДИКТ и в implement, и в review выносит opus-`adjudicator`,
а не Sonnet-родитель — так sonnet-оркестратор не роняет качество. coder — sonnet. Дороже/тщательнее
разово — подними оркестратор до `--model opus` (или `--effort xhigh`); дешевле — оставь sonnet, опусти `--effort`.
Порядок: не запускать до фазы C (CI настоящий + required checks в ruleset, и ruleset НЕ требует
аппрува — см. §Гейт). Конвейер: `/feeder docs/` → ручной триаж → `fleet-preflight` (разово) → обёртка.
Стоп — issue с лейблом `fleet-stop`.


## Переход на одного агента (человек, разово)

Раньше было два-три аккаунта с кросс-ревью; стало — один. Скиллы переключены, но GitHub-сторону
(зона человека, флот туда не пишет) надо подготовить РУКАМИ, иначе single-agent либо не вмержит
ничего, либо застрянет на legacy-PR.

1. **Ruleset** (Settings → Rules / branch protection ветки `main`):
   - СНЯТЬ «Require a pull request before merging → Require approvals» (поставить 0) и «Require approval
     of the most recent reviewable push». Один аккаунт не может за-аппрувить свой PR — при требовании
     аппрува КАЖДЫЙ PR висит вечно.
   - ОСТАВИТЬ включёнными: «Require status checks to pass» со ВСЕМИ джобами (`lint typecheck test build
     frontend`), «Require conversation resolution before merging», squash-only,
     блокировку force-push. Это и есть весь биндящий гейт.
2. **Лейбл** `reviewed-armed` — создать в репо (Issues → Labels), если ещё нет; на нём завязан весь
   single-agent каскад select.md.
3. **Legacy In-Review PR** (открыты в кросс-аккаунт-эпоху, ждут второго аккаунта, который не придёт):
   на каждом снять протухшие старые ревью, чтобы `reviewDecision` перестал мешать
   (`gh pr review <N> --dismiss -m "single-agent cutover"` или включить в ruleset «Dismiss stale pull
   request approvals when new commits are pushed»), убедиться, что на PR нет `needs-work`/`reviewed-armed`
   — тогда новый каскад подберёт их в REVIEW и прогонит свежий self-review. Их не нужно закрывать —
   они дойдут до мержа по новым правилам.
4. Запустить на ОДНОЙ машине: `fleet-preflight` (разово) → петля `/work-cycle` (см. §Запуск).


## Что держать под контролем

- `PROJECTS_TOKEN` обязателен, иначе движение карточек молча мертво. Токены истекают.
- Ruleset под single-account: require-approvals и «approval of most recent push» ВЫКЛЮЧЕНЫ (иначе свой
  PR не вмержить — GitHub запрещает само-аппрув), required CI-checks + require-conversation-resolution
  ВКЛЮЧЕНЫ. `fleet-preflight` напоминает и (если может прочитать ruleset) FAIL'ит при требовании аппрува.
- Один аккаунт: `gh auth`/git и токен MCP = ОДИН и тот же аккаунт (`fleet-preflight` FAIL при gh≠MCP).
- Legacy In-Review PR из кросс-аккаунт-эпохи: снять протухшие старые ревью (`gh pr review --dismiss` или
  включить «dismiss stale reviews on push»), чтобы `reviewDecision` сбросился, и прогнать через новый
  review.md — иначе старый `APPROVED`/`CHANGES_REQUESTED` мешает каскаду (см. ниже «Переход…»).
- assignee на PR через MCP — проверить на живом PR, что `update_issue` с `assignees` реально назначает.
- Точный login бота Copilot подсмотреть на реальном PR (нужен review.md).
- Качество бэклога = качество спеки: размытые/неполные критерии — `needs-human` (полнота критериев на
  человеке: единственный не-Claude ревьюер, см. §Гейт слой 5).
- `frontend-conventions` — стартовый канон, уточнить под реальную структуру `frontend/`.