# Add a CLI command

Use this when an operator or a script needs a command. There are two kinds:

- **Remote** (almost all): a Connect client of a running server. It calls an RPC, so the RPC comes first ([rpc.md](rpc.md)). The server checks permission and reach; the command only sends ids and prints the answer.
- **Operator**: runs on the server with database access. It boots the `App` (no install URL, scheduler or consumer) and calls a service directly as the operator. It refuses `--url`, `--token`, `--token-file` and `--profile`. Today: `user set-password`, `member add`.

A domain's command tree is its `internal/<domain>/cli.go`. `internal/cli` is only the shell (root, flags, config, URL, token, profile and session-file resolution, and the adapters that feed each `NewCmd`). Never add a command file to `internal/cli`.

## Plan

- [ ] Remote or operator? Pick operator only for work that must run without a signed-in caller (recovery, bootstrap).
- [ ] Remote: which RPC does it call? It adds no logic the RPC lacks.
- [ ] Command path and help group: `<domain> <verb>`, in the group of its siblings (`domain`, `tenancy`, `auth` or `runtime`).
- [ ] Arguments: the target's public id as the positional arg (`get <public-id>`); other ids as `--org-id`, `--project-id` flags. No UUIDs, no slugs as ids.
- [ ] Output: a text line or a `render.Table` for people, `render.JSON` of a camelCase map for scripts.
- [ ] Does it remove something a person cannot undo? Some commands ask `prompt.YesNo` with a `--yes` flag (`member remove`, `member leave`, `invite revoke`).
- [ ] Does it read a secret? Use `internal/prompt` (`Secret`, `NewSecret`) with a `--…-stdin` flag, never a plain flag.

## Build

1. **Remote: add the subcommand** to `internal/<domain>/cli.go` and list it in `NewCmd`. Every command is a factory taking the domain's `ClientFn`, so each test gets a fresh tree. Shape, copied from `agent`'s `newGetCmd`:

   ```go
   func newGetCmd(clientFn ClientFn) *cobra.Command {
       return &cobra.Command{
           Use:   "get <public-id>",
           Short: "Get an agent by id",
           Args:  cobra.ExactArgs(1),
           RunE: func(cmd *cobra.Command, args []string) error {
               client, err := clientFn(cmd)
               if err != nil {
                   return err
               }
               resp, err := client.GetAgent(cmd.Context(), connect.NewRequest(&agentv1.GetAgentRequest{Id: args[0]}))
               if err != nil {
                   return err
               }
               a := resp.Msg.GetAgent()
               if isJSON(cmd) {
                   return render.JSON(cmd.OutOrStdout(), agentMap(a))
               }
               return render.Table(cmd.OutOrStdout(), []string{"ID", "PROJECT_ID", "SLUG", "NAME"},
                   [][]string{{a.GetId(), a.GetProjectId(), a.GetSlug(), a.GetName()}})
           },
       }
   }
   ```

   Required flags use `cmd.MarkFlagRequired`. Return errors unchanged: the root prints them and `ExitCodeFor` (`internal/cli/exit.go`) maps the code's kind to the exit code (invalid 2, not found 3, already exists 4, unauthenticated 5, else 1).

2. **Remote, new domain only: wire it** in `NewRootCmd` (`internal/cli/root.go`): a `<domain>ClientFn(bootClient)` adapter that returns the field of `apiclient.Client`, `markClientTree(cmd)`, and `root.AddCommand`. Add the client to `internal/apiclient/client.go` if missing. `connectClient` already resolves the URL (`--url` > `OAGENT_URL` > profile > `http.baseURL` > default) and the credential (`--token` > `--token-file` > `OAGENT_TOKEN` > the profile's session).

3. **Operator: three pieces.**
   - The domain's `cli.go` declares a function type and the command; it never sees the `App`:

     ```go
     // SetPasswordFn sets a user's password and signs out their sessions; the caller boots the server graph.
     type SetPasswordFn func(ctx context.Context, email, password string) error
     ```

   - `internal/boot/app.go` gets the `App` method, which calls the service as the operator (`AddMember` passes `member.Requester{Operator: true}`).
   - `internal/cli/root.go` adds the method to the `App` interface, an adapter that runs it through `withOperatorApp`, and wraps the command in `markOperator(...)`, which makes `refuseRemoteFlags` reject the remote flags.

4. **Tests.**
   - Command: `internal/<domain>/cli_test.go` runs the tree against a real `controlplane.New` behind `httptest` with `cptest.OperatorOptions()` and fake stores (`runAgent`, `agentClient`). Cover text and JSON output and an error.
   - Root: a new group goes in `remoteCommands` (`internal/cli/clientfn_root_test.go`); an operator command gets a `…RefusesRemoteFlags` test like `TestUserSetPassword_RefusesRemoteFlags`.
   - Public ids: add a JSON-output case to `TestCLI_JSONHoldsNoUUID` (`internal/boot/cli_publicid_integration_test.go`).

5. **Docs.** Name the command in the "Connect + CLI" column of `internal/boot/README.md`.

6. **Gates.** `make check`, `make comment-check`, `make cover`.

## Review

- [ ] The command lives in `internal/<domain>/cli.go`; `internal/cli` changed only in its adapters and root list.
- [ ] The domain package imports no `internal/cli`, `internal/boot` or `internal/controlplane` (tests may use `internal/controlplane` to stand up a server).
- [ ] A remote command calls exactly one RPC path and adds no business rule of its own.
- [ ] An operator command boots the `App` through `withOperatorApp`, is wrapped in `markOperator`, and is tested to refuse `--url`.
- [ ] Ids in and out are public ids; JSON keys are camelCase.
- [ ] Output goes to `cmd.OutOrStdout()`, questions to stderr; nothing prints a secret.
- [ ] Errors keep their app error code, so exit codes stay stable.

## Reference code

- [internal/agent/cli.go](../../internal/agent/cli.go): `ClientFn`, `NewCmd`, `newGetCmd`, `agentMap`
- [internal/org/member/cli.go](../../internal/org/member/cli.go): `AddFn`, `NewAddCmd`, `prompt.YesNo` with `--yes`
- [internal/user/cli.go](../../internal/user/cli.go): `SetPasswordFn`, `prompt.NewSecret`
- [internal/cli/root.go](../../internal/cli/root.go): `NewRootCmd`, `App`, `connectClient`, `withOperatorApp`, `markOperator`, `refuseRemoteFlags`
- [internal/cli/profile.go](../../internal/cli/profile.go): `resolveRemote`
- [internal/cli/exit.go](../../internal/cli/exit.go): `ExitCodeFor`
- [internal/apiclient/client.go](../../internal/apiclient/client.go): `Client`, `New`, `Credential`
- [internal/render/render.go](../../internal/render/render.go): `Detect`, `JSON`, `Table`
- [internal/prompt/prompt.go](../../internal/prompt/prompt.go): `YesNo`, `Secret`, `NewSecret`
- [internal/boot/app.go](../../internal/boot/app.go): `SetPassword`, `AddMember`
- [internal/agent/cli_test.go](../../internal/agent/cli_test.go): `runAgent`, `agentClient`
- [internal/boot/cli_publicid_integration_test.go](../../internal/boot/cli_publicid_integration_test.go): `TestCLI_JSONHoldsNoUUID`
