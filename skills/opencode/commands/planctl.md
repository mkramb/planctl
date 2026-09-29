---
description: Run a planctl command
---

Run exactly this command, with the arguments the user gave, and nothing else:

```sh
planctl $ARGUMENTS
```

Report its output. Do not substitute, skip, or reinterpret the command. In
particular, `planctl review` publishes the plan and blocks until the review is
approved or changes are requested — do not start implementing while it is
blocked.
