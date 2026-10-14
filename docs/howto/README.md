# How-to recipes

Pick the recipe for your change. Each one has a **Plan** checklist (decisions to make first), **Build** steps (the files to touch, in order) and a **Review** checklist, and links to code that already follows the pattern. Many changes need several recipes: a new RPC usually also needs a service method, an error code and a console page.

| I want to…                                             | Recipe                              |
| ------------------------------------------------------ | ----------------------------------- |
| add a domain (a new package under `internal/`)         | [module](module.md)                 |
| add business logic to a domain                         | [service-method](service-method.md) |
| read or write the database                             | [store-method](store-method.md)     |
| expose an identifier anywhere a user or client sees it | [public-ids](public-ids.md)         |
| add or change an API call (Connect RPC)                | [rpc](rpc.md)                       |
| expose an RPC as an MCP tool                           | [mcp-tool](mcp-tool.md)             |
| add a CLI command                                      | [cli-command](cli-command.md)       |
| add an error code a user can see                       | [error-code](error-code.md)         |
| add a config key                                       | [config-key](config-key.md)         |
| change the database schema                             | [migration](migration.md)           |
| add a background worker or job                         | [worker](worker.md)                 |
| send a new webhook event                               | [webhook-event](webhook-event.md)   |
| rate-limit an action                                   | [rate-limit](rate-limit.md)         |
| add a page before sign-in (templ)                      | [web-page](web-page.md)             |
| add a console page (signed-in UI)                      | [console-page](console-page.md)     |

Gates for every change are in [AGENTS.md](../../AGENTS.md#gates).
