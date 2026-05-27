# Cloudflare Login Fix for cf-tool

## Background

Codeforces added Cloudflare protection to its website. The original `cf-tool` login
mechanism used a plain HTTP POST to `/enter`, which cannot pass Cloudflare's JavaScript
challenge. As a result, the `cf config` → login flow stopped working.

## Root Cause

The original `Login()` function in `client/login.go`:

1. Creates a bare `http.Client`
2. GETs `/enter` to fetch a CSRF token
3. POSTs credentials directly

Cloudflare intercepts step 2 and returns a JavaScript challenge page instead of the
real login page, so no CSRF token is found and login fails immediately.

## Fix Overview

Rather than automating the Cloudflare challenge (which requires a headless browser), the
fix takes a simpler and more robust approach: **let the user log in through their normal
browser, then hand the resulting cookies to cf-tool**.

Once the browser has passed the Cloudflare challenge and completed the Codeforces login,
the cookie jar contains everything needed (including `cf_clearance`). cf-tool injects
those cookies and verifies the session by checking the homepage for the logged-in handle.

## Changes

### `go.mod`

| Before | After |
|--------|-------|
| `module sg.com/sg/cf-tool` | `module gitcode.com/sheng_wang/cf-tool_modify` |
| `require gitcode.com/sheng_wang/cf-tool_modify v1.0.0` | *(removed)* |

The module name was mismatched with all local import paths, causing Go to resolve every
internal package from the stale `vendor/` snapshot instead of the local source files.
Restoring the original module name fixes the build.

### `client/login.go`

Two new functions added:

**`LoginWithCookies(cookieStr string) error`**
- Parses a semicolon-separated cookie header string into `[]*http.Cookie`
- Injects the cookies into a fresh `cookiejar.Jar` for the Codeforces host
- GETs the homepage and calls `findHandle()` to confirm the session is valid
- Saves the session on success

**`ConfigLoginWithCookies() error`**
- Interactive entry point: prints step-by-step instructions for the user
- Reads the pasted cookie string from stdin
- Delegates to `LoginWithCookies()`

### `cmd/config.go`

Added menu option `1) login with browser cookies (Cloudflare)` and shifted the
remaining options up by one (total options: 9).

## How to Use

```
cf config
```

Select option `1`, then follow the on-screen steps:

1. Open `https://codeforces.com` in your browser and log in normally.
2. Open DevTools (`F12`) → **Application** → **Cookies** → `https://codeforces.com`.
3. Collect all `Name=Value` pairs and join them with `; ` into a single line.
   - Tip: the browser extension **Cookie-Editor** → Export → Header String does this
     in one click.
4. Paste the string into the terminal and press Enter.

cf-tool will verify the session and save it. All subsequent commands (`cf submit`,
`cf parse`, etc.) reuse the saved cookies automatically.

## Maintenance

Cloudflare's `cf_clearance` cookie typically expires after a few days. When cf-tool
starts returning login errors, simply repeat the steps above to refresh the cookies.
