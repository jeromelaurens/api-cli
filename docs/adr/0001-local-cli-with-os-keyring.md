# Local CLI with OS keyring

Accepted.

The project starts as a local `api` CLI rather than an MCP server or a third-party broker integration such as Apideck, Zapier, or Pipedream. Non-secret API profiles live in the local profile catalog, while credentials live in the operating-system keyring through `github.com/zalando/go-keyring`; this keeps credentials out of model-visible configuration, agent context, shell arguments, logs, and third-party platforms at the cost of less automatic API discovery and no remote broker workflow in v1.
