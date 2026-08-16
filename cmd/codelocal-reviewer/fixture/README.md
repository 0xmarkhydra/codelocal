# CodeLocal Reviewer Project

This is a deterministic sample project used only for OpenAI plugin review.

It contains a tiny calculator module with one intentional bug: division by zero should throw a clear error, but the initial implementation does not.

Useful reviewer workflows:

- inspect the project structure and explain the module;
- find the failing behavior from tests;
- fix the bug in `src/calculator.js`;
- run `npm test` and verify the change;
- inspect Git status/diff;
- confirm path-escape and force-push protections are enforced.

No production credentials, customer data, private repositories or external accounts are present in this fixture.
