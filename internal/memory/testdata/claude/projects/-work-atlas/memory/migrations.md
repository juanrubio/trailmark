---
name: Migration ordering
description: Apply schema changes before code deploys
metadata:
  type: project
  originSessionId: sess-atlas-1
---

# Migration ordering

Apply schema changes before code deploys. The quokka marker lives here.
Always run the backfill before flipping the reader flag.
