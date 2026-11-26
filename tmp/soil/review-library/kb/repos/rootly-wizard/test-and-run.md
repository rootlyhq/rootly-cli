- Test: `npm test` (node --test).
- The interactive wizard needs a TTY: `node ./src/cli.js`. Agent mode:
  `node ./src/cli.js action <name> '<json>'` with `ROOTLY_TOKEN` set. API and OAuth hosts default to
  api.rootly.com with no env override.

_Seeded from Devin's rootlyhq/rootly-wizard blueprint (test, run)._
