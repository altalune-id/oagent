# Add a pre-sign-in web page

A server-rendered templ page that a person opens **before** they are signed in: sign in, install, forgot and reset password, the invite landing, and the legal pages (`/terms`, `/privacy`, `/welcome`).

## When to use

- The page must work with no session: a sign-in flow, a link from an email, a legal document.
- The handler calls a domain **service** directly. Only pre-sign-in pages may do that.

**Anything a signed-in person uses goes in the console, not templ.** See [console-page.md](console-page.md). Signed-in surfaces call the `controlplane` handlers (permission and reach checks); a templ page would skip them.

## Plan

- [ ] The page is pre-sign-in. If not, stop and use [console-page.md](console-page.md).
- [ ] Which domain owns it: `internal/<domain>/web.go` (handlers) and `internal/<domain>/views` (templ). Never `internal/web`.
- [ ] Routes and methods (`GET` shows, `POST` acts), under the base path (`Kit.Path`).
- [ ] Does it exist only with local login? Then it is built after the `LocalLogin` check in `buildRoutes`.
- [ ] Rate limits for every form that takes an email, a password or a token ([rate-limit.md](rate-limit.md)).
- [ ] New service method or error code? Do [service-method.md](service-method.md) or [error-code.md](error-code.md) first.
- [ ] Must a signed-in person who has not accepted the terms reach it? Then it joins the `WelcomeGate` skip list.

## Build

1. **View.** Add `internal/<domain>/views/<noun>.templ` (package `views`). The component is a plain noun with an `XxxData` input. It imports only `internal/web/layout`, never its own domain.

   ```templ
   package views

   import "altalune.id/oagent/internal/web/layout"

   // ThingData is the thing form's state.
   type ThingData struct {
   	Email string
   	Error string
   }

   templ Thing(d layout.LayoutData, v ThingData) {
   	@layout.Layout(d) {
   		@layout.Card(d.Title) {
   			if v.Error != "" {
   				<div role="alert" class="alert alert-error alert-soft">{ v.Error }</div>
   			}
   			<form method="post" action={ d.Href("/thing") } class="flex flex-col gap-3">
   				<label class="floating-label">
   					<span>Email</span>
   					<input class="input w-full" type="email" name="email" value={ v.Email } placeholder="Email" required/>
   				</label>
   				<button class="btn btn-primary" type="submit">Continue</button>
   			</form>
   		}
   	}
   }
   ```

   Links use `d.Href(...)`, assets `d.Static(...)`. A new password input uses `layout.PasswordField`; terms consent uses `layout.Consent`. Copy is plain English in the view (templ pages have no i18n).

2. **Generate.** `make templ` (or `make generate`). Commit the `*_templ.go` files.
3. **Handlers.** In `internal/<domain>/web.go`: an options struct, `type Web`, `NewWeb`, `Register(mux *http.ServeMux)`.

   ```go
   // WebOpts configures the thing pages.
   type WebOpts struct {
   	Kit     layout.Kit
   	Service *Service
   	Limits  auth.Limits
   }

   // Web serves /thing.
   type Web struct{ o WebOpts }

   // NewWeb binds the thing pages to their options.
   func NewWeb(o WebOpts) *Web { return &Web{o: o} }

   // Register mounts the thing pages on mux.
   func (h *Web) Register(mux *http.ServeMux) {
   	mux.HandleFunc("GET "+h.o.Kit.Path("/thing"), h.getThing)
   	mux.HandleFunc("POST "+h.o.Kit.Path("/thing"), h.postThing)
   }

   func (h *Web) postThing(w http.ResponseWriter, r *http.Request) {
   	email := strings.TrimSpace(r.PostFormValue("email"))
   	if err := h.o.Limits.LimitIP(r, ThingIPRule()); err != nil {
   		auth.WriteLimited(w, r, h.o.Kit, "Thing", err)
   		return
   	}
   	if err := h.o.Service.Do(r.Context(), email); err != nil {
   		h.o.Kit.Fail(w, r, "web: thing", err)
   		return
   	}
   	h.o.Kit.Render(w, r, http.StatusOK, views.Thing(h.o.Kit.Layout("Thing"), views.ThingData{}))
   }
   ```

   - `Kit.Render` writes the page with a status; `Kit.Fail` logs the error and shows the generic 500 page. The error never reaches the response.
   - A known user error re-renders the form with `Error` and a 4xx status (see `postLogin`, `postWelcome`).
   - `return_to` goes through `layout.SanitizeReturnTo` (into the form) and `layout.ResolveReturnTo` (before the redirect).
   - Answer the same for known and unknown emails.

4. **Rate limits.** Define `XxxRule() ratelimit.Rule` funcs next to the handlers, count with `auth.Limits` (`LimitIP`, `LimitEmail`) and refuse with `auth.WriteLimited` (429 + `Retry-After`, or 503 when the store fails). See [rate-limit.md](rate-limit.md).
5. **Wire.** Add `<domain>.NewWeb(...)` to the slice in `buildRoutes` ([internal/boot/http.go](../../internal/boot/http.go)). Pass `kit` and the services from `Services`.
6. **Gates.**
   - Routes are open to signed-out callers; the handler decides who may act.
   - Cross-origin `POST`s are refused by `http.CrossOriginProtection` (set in `web.NewServer`), so forms carry no CSRF token.
   - A page that a person must reach before accepting the terms joins `skipped` in `WelcomeGate` ([internal/legal/web.go](../../internal/legal/web.go)).
   - A page that renders a token from a URL sets `Referrer-Policy: no-referrer` (see `resetLimited`).

7. **CSS and JS.** Use Tailwind and daisyUI classes. `ui/web/app.css` scans `internal/**/*.templ`, so run `make web-css` after you add classes. The only script is the password bundle (`ui/web/src/password`, `make web-js`). `make web` (the `@oagent/web` scripts `build` and `build:js`) builds both into `internal/web/static/` and writes a `.br` and `.gz` next to each (`ui/scripts/compress.mts`); `/static` serves them by `Accept-Encoding` with an `ETag` and a one-hour cache. These built files are not committed: Go tests that read them skip locally when they are missing and fail when `CI` is set.
8. **Tests.** `internal/<domain>/web_test.go` drives the page through `web.NewServer(web.ServerOpts{Routes: ...})` or `httptest`. Add `web_limits_test.go` for the limits. Add the path to `TestLockdown_NoCredential` in [internal/boot/lockdown_integration_test.go](../../internal/boot/lockdown_integration_test.go) when it must stay open.
9. **Docs.** Update the "Web routes" column and the open paths in [internal/boot/README.md](../../internal/boot/README.md).

## Review

- [ ] The page is pre-sign-in. Nothing signed-in was added as templ.
- [ ] The view lives in `internal/<domain>/views`, has an `XxxData` input and imports only `internal/web/layout`.
- [ ] `internal/web` imports no domain; the domain imports no `internal/web` other than `layout`.
- [ ] Every route uses `Kit.Path`; every link `d.Href`; every asset `d.Static`.
- [ ] Every form that takes an email, password or token is rate limited and answers the same for known and unknown emails.
- [ ] No error text, token or email leaks into the response, a log line or a `Referer`.
- [ ] `return_to` is sanitized before it is rendered and resolved before it is followed.
- [ ] `*_templ.go` regenerated; `buildRoutes` and the boot README updated.
- [ ] Gates pass: `make generate`, `make web`, `make check`.

## Reference code

- [internal/auth/web.go](../../internal/auth/web.go): `Web`, `Register`, `getLogin`, `postLogin`, `postForgot`, `resetLimited`
- [internal/auth/views/login.templ](../../internal/auth/views/login.templ): `Login`, `LoginData`
- [internal/auth/views/forgot.templ](../../internal/auth/views/forgot.templ): `Forgot`, `ForgotData`
- [internal/auth/limits.go](../../internal/auth/limits.go): `Limits`, `LoginIPRule`, `WriteLimited`
- [internal/legal/web.go](../../internal/legal/web.go): `WebOpts`, `Web`, `WelcomeGate`, `postWelcome`
- [internal/legal/views/welcome.templ](../../internal/legal/views/welcome.templ): `Welcome`, `WelcomeData`
- [internal/org/invite/web.go](../../internal/org/invite/web.go): `WebOpts`, `LandingRule`, `SubmitRule`
- [internal/web/layout/layout.go](../../internal/web/layout/layout.go): `LayoutData`, `Kit`, `Render`, `Fail`
- [internal/web/layout/layout.templ](../../internal/web/layout/layout.templ): `Layout`, `Card`
- [internal/web/web.go](../../internal/web/web.go): `Registrar`, `NewServer`
- [internal/boot/http.go](../../internal/boot/http.go): `buildRoutes`
- [ui/web/app.css](../../ui/web/app.css), [ui/web/vite.config.ts](../../ui/web/vite.config.ts)
