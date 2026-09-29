---
description: Publish files for review as a GitHub pull request and address feedback
---

Run exactly this command, with the arguments the user gave, and nothing else:

```sh
planctl $ARGUMENTS
```

`planctl` publishes the given files (or a folder, or every changed file when no
arguments are given) as a GitHub pull request and waits for review. Then follow
this loop until the review is approved:

1. Read the command's output.
2. If it reports the review is approved ("allowed" is true): stop.
3. If it includes comments (feedback):
   - If reviewers were requested (`--reviewer` in $ARGUMENTS), ask the user about
     each comment — apply or ignore — and apply only the changes they accept.
   - Otherwise, revise the files automatically to address every comment.
   - Run `planctl $ARGUMENTS` again and go back to step 1.

Do not mark a comment resolved unless the file is actually changed.
