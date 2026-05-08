# DShare

DShare is a Discord-gated relay in front of `new-api`, plus a contribution portal for adding verified DeepSeek accounts into `ds2api`.

## What It Does

- Discord OAuth login, with a local admin password fallback.
- Local admin fallback is configured through `ADMIN_USERNAME` / `ADMIN_PASSWORD`; change both values before production.
- Issues DShare API keys to logged-in users.
- Proxies model API traffic to `new-api` while preserving request and response shape, including streaming responses.
- Applies per-user request limits before traffic reaches `new-api`. New users default to 5 RPM and 2500 accepted requests per UTC day.
- Lets users import `账号:密码` DeepSeek credentials in bulk.
- Adds each account to `ds2api`, calls `ds2api` account testing, and only counts newly verified accounts as valid contribution.
- Each new valid contribution increases that user's limit by 1 RPM and 100 daily requests.
- API request logs store path, query, status, duration, bytes, IP, User-Agent, Referer, and request id for audit/future rules.
- `/admin` dashboard shows users, Discord info, valid upload count, total calls, and editable limits.

## Run Locally

```bash
cp .env.example .env
docker compose up --build
```

Open `http://localhost:12399`.

- `/` is the public user entry and only shows Discord login before login.
- `/admin` is the admin entry and uses `ADMIN_USERNAME` / `ADMIN_PASSWORD`.

Discord callback URL:

```text
http://localhost:12399/api/auth/discord/callback
```

API clients should use this service as the base URL and a DShare key as the API key:

```text
http://localhost:12399/v1/chat/completions
Authorization: Bearer dsh-...
```

The service rewrites upstream auth to `NEW_API_KEY`.

## Configure new-api

1. Open your running new-api dashboard.
2. Create or choose a token that DShare will use as the upstream token. It must have access to the models/groups you want users to call.
3. Copy the full token, usually `sk-...`.
4. Set these values in `.env`:

```env
NEW_API_BASE_URL=http://你的-new-api-地址
NEW_API_KEY=sk-你的-new-api-token
```

If new-api runs on the same Docker host but outside this compose project, `host.docker.internal` may work on Docker Desktop. On Linux servers, using the server LAN/public address or putting both services on the same Docker network is more reliable.

Example:

```env
NEW_API_BASE_URL=http://new-api:3000
NEW_API_KEY=sk-xxxxxxxx
```

After changing `.env`:

```bash
docker compose up -d --force-recreate
```

## Configure ds2api

1. Open your running ds2api admin panel or check the ds2api server environment.
2. Find the ds2api admin key. In ds2api this is normally `DS2API_ADMIN_KEY`; if it was not set, ds2api may use the insecure default `admin`.
3. Confirm ds2api admin login works:

```bash
curl -X POST http://你的-ds2api-地址/admin/login \
  -H 'Content-Type: application/json' \
  --data '{"admin_key":"你的DS2API_ADMIN_KEY","expire_hours":24}'
```

The response should contain `success: true` and a `token`.

4. Set these values in `.env`:

```env
DS2API_BASE_URL=http://你的-ds2api-地址
DS2API_ADMIN_KEY=你的DS2API_ADMIN_KEY
DS2API_AUTO_PROXY_ENABLED=true
DS2API_AUTO_PROXY_TYPE=socks5
DS2API_AUTO_PROXY_HOST=172.20.0.1
DS2API_AUTO_PROXY_PORT=21345
DS2API_AUTO_PROXY_USERNAME_TEMPLATE=Default.{local}
DS2API_AUTO_PROXY_PASSWORD=你的代理密码
DS2API_AUTO_PROXY_NAME_TEMPLATE=resin-{local}
```

Example:

```env
DS2API_BASE_URL=http://host.docker.internal:5001
DS2API_ADMIN_KEY=your-ds2api-admin-key
```

`DS2API_AUTO_PROXY_*` should match the auto-proxy settings you would use in ds2api's own batch account import page. DShare sends email accounts through ds2api's `/admin/accounts/bulk-import` endpoint so ds2api creates the per-account proxy and binds `proxy_id` the same way its own UI does.

After changing `.env`:

```bash
docker compose up -d --force-recreate
```

## Discord OAuth

In the Discord Developer Portal, create an OAuth2 application and add this redirect URL:

```text
http://你的域名或IP:12399/api/auth/discord/callback
```

Then set:

```env
APP_BASE_URL=http://你的域名或IP:12399
DISCORD_CLIENT_ID=你的Discord Client ID
DISCORD_CLIENT_SECRET=你的Discord Client Secret
```

## Notes

- `DS2API_BASE_URL` and `DS2API_ADMIN_KEY` must point to an existing ds2api admin API.
- Account contribution is counted only after `POST /admin/accounts/test` returns success.
- Failed validation attempts are recorded for audit but do not increase valid uploads.
