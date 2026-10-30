# Security

Report a vulnerability privately, through GitHub:
[Report a vulnerability](https://github.com/jvrsantacruz/agent-status/security/advisories/new).
Please do not open a public issue for it.

You get an answer within 7 days. A fix is released as a new version, and the advisory credits you
unless you ask otherwise.

Only the latest release gets fixes.

What agent-status is exposed to: it reads events from a FIFO that only your user can write, and
writes files that only your user can read. It opens no network connection.
