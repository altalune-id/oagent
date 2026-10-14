# Git hooks

[husky](https://typicode.github.io/husky/) installs these hooks on `pnpm install` (the `prepare` script).
They run the same checks as CI, so a hook that fails here would fail CI too.

| Hook         | What runs                              | Fails when                                                                                                                                                                                                       |
| ------------ | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pre-commit` | `lint-staged` on the staged files only | a staged `.go` file is not gofmt-formatted; ESLint can't fix a JS/TS/Vue/JSON/Markdown/CSS problem (it fixes and re-stages the rest); yamllint, hadolint, actionlint, shellcheck or `buf lint` reports a problem |
| `pre-push`   | signed-commit check                    | an outgoing commit has no signature                                                                                                                                                                              |
| `pre-push`   | `make lint`, `make comment-check`      | golangci-lint, ESLint, the config linters or comment-lint report a problem                                                                                                                                       |
| `pre-push`   | `gen-config-example --check`           | `.env.reference` / `config.reference.yml` drifted from the config structs (`make config-examples`)                                                                                                               |
| `pre-push`   | `go test -race -shuffle=on ./...`      | a unit test fails                                                                                                                                                                                                |
| `pre-push`   | `buf breaking` against `origin/main`   | a proto change breaks the API (skipped when there is no `origin/main`)                                                                                                                                           |

## Node version

Both hooks stop when `node` is older than 24. Git and git GUIs don't run `nvm use`, so make Node 24 the
default: `nvm alias default 24`.

## Optional tools

`scripts/lint/tool.sh` runs yamllint, hadolint, actionlint, shellcheck and goreleaser from the installed
binary, else from their pinned Docker image, else prints a note and skips. CI sets `TOOL_STRICT=1`, so CI never
skips. Install any of them to make the hooks faster, for example:

```sh
brew install yamllint hadolint actionlint shellcheck goreleaser
```

`TOOL_DOCKER=1` forces the Docker images (the versions CI uses).

## Signed commits

Every commit must be signed with GPG or SSH. The pre-push hook refuses an unsigned commit. When this machine
can't verify a signature (the GPG key isn't in your keyring, or SSH signing without an allowed-signers file),
the hook only prints a note: the commit is signed, and GitHub verifies it.

```sh
git config commit.gpgsign true
git config tag.gpgsign true
# GPG
git config user.signingkey <key-id>
# or SSH
git config gpg.format ssh
git config user.signingkey ~/.ssh/id_ed25519.pub
# let git verify SSH signatures locally
echo "$(git config user.email) $(cat ~/.ssh/id_ed25519.pub)" >> ~/.config/git/allowed_signers
git config gpg.ssh.allowedSignersFile ~/.config/git/allowed_signers
```

## Commit messages

Conventional Commits, one line. Use the template with `git config commit.template .gitmessage.txt`.

## Skipping

Don't use `--no-verify`. If a hook fails, fix the cause; CI runs the same checks. Hooks not running?
`pnpm install` reinstalls them.
