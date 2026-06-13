---
name: work-cycle
description: One cycle of an autonomous worker — pick work from the GitHub queue, do it, exit.
disable-model-invocation: true
---
Run EXACTLY ONE cycle and exit. Hold no state in memory — everything lives in GitHub.

1. Read `steps/select.md` and pick ONE unit of work.
   - If there's no work → print exactly `WORK_QUEUE_EMPTY` and exit. (The outer wrapper stops the loop on this word.)
2. Depending on the type of the chosen work, run EXACTLY ONE step:
   - changes requested (needs-work) → `steps/address.md`
   - review a PR → `steps/review.md`
   - new issue → `steps/implement.md`
3. After finishing the unit — exit the process. Do NOT take a second unit in the same invocation (the next invocation starts with a clean context).
