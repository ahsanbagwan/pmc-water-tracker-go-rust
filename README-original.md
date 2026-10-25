# Where the Water Reaches

A locality-level tracker of Pune Municipal Corporation (PMC) water status: where municipal
piped supply reaches, which areas depend on tankers, and where pipeline work is underway.

**Why this exists:** PMC doesn't publish a public, locality-level, machine-readable feed of
water-supply status. This project fills that gap with a curated dataset — every entry cites
where the claim comes from and when it was last checked — plus a review queue for citizen
corrections, so the data can stay current without pretending to be a live official source.

## Code quality (SonarQube Cloud + JaCoCo)

**JaCoCo (coverage) needs no account and works right now:**
```bash
cd backend
mvn test
```
Then open `backend/target/site/jacoco/index.html` in a browser for a per-class/package
coverage report -- useful on its own, independent of everything below.

**SonarQube Cloud (static analysis + coverage dashboard) needs one-time account setup:**
1. Sign up at [sonarcloud.io](https://sonarcloud.io) with your GitHub account -- free for
   public repos.
2. Create an organization (it'll suggest one from your GitHub username) and import this repo
   as a new project.
3. From the project's **Administration → Analysis Method**, note your **organization key**
   and **project key**.
4. Replace `REPLACE_WITH_YOUR_ORG_KEY` and `REPLACE_WITH_YOUR_PROJECT_KEY` in
   `backend/pom.xml`'s `<properties>` block with those real values.
5. Generate a token: **My Account → Security → Generate Token**.
6. Add it as a GitHub Actions secret: repo **Settings → Secrets and variables → Actions →
   New repository secret**, name it `SONAR_TOKEN`, paste the token.
7. Push to `main` (or open a PR) -- `.github/workflows/build-and-analyze.yml` runs tests,
   generates the JaCoCo report, and sends both to SonarQube Cloud automatically. Check the
   Actions tab for the run, then your SonarCloud project dashboard for results.

**To run the same analysis locally** (useful for checking before you push), once the
placeholders above are filled in:
```bash
cd backend
mvn verify org.sonarsource.scanner.maven:sonar-maven-plugin:sonar -Dsonar.token=<your-token>
```

## Project layout

```
backend/    Spring Boot 3 REST API (Java 21) -- dev profile: H2 in-memory; prod profile: MySQL (Aiven)
frontend/   React + Vite app: Leaflet map + searchable/filterable list
```

## Running the backend

Requires JDK 21 and Maven.

```bash
cd backend
mvn spring-boot:run
```

Starts on `http://localhost:8080` using the `dev` profile (in-memory H2, no setup, seeds fresh
on every restart from `LocalityDataInitializer`). See "Deploying with AWS RDS" below to run
against real MySQL instead.

Key endpoints:

| Method | Path                                | Purpose                                      |
|--------|-------------------------------------|-----------------------------------------------|
| GET    | `/api/localities`                   | List localities. Filters: `?status=`, `?search=` |
| GET    | `/api/localities/{id}`              | One locality                                  |
| POST   | `/api/localities`                   | Add a curated (verified) entry                |
| POST   | `/api/localities/reports`           | Submit a citizen report (goes in as unverified) |
| GET    | `/api/localities/reports/pending`   | List reports awaiting review                  |
| POST   | `/api/localities/reports/{id}/approve` | Publish a pending report                   |
| PUT    | `/api/localities/{id}`              | Edit an entry                                 |
| DELETE | `/api/localities/{id}`              | Remove an entry                               |

`status` is one of `MUNICIPAL`, `TANKER_DEPENDENT`, `MIXED`, `PIPELINE_IN_PROGRESS`.

### Auth

Write access requires logging in and using the returned token — not per-user Basic auth on
every request. Two roles:

| Role   | Can do |
|--------|--------|
| ADMIN  | Everything: create/edit/delete localities, approve reports, manage user accounts |
| EDITOR | Create/edit localities, approve citizen reports -- cannot delete entries or manage users |

`GET` requests and `POST /api/localities/reports` (citizen report submission) stay open to
everyone, no login required.

**First login:** on first startup, if no users exist yet, an admin account is created
automatically from `ADMIN_USERNAME`/`ADMIN_PASSWORD` env vars (falls back to
`admin` / `change-me-now` locally if unset). **Set real values for these on Render** under
your service's Environment tab before it's public — the app refuses to start under the `prod`
profile while `ADMIN_PASSWORD` is still the default.

**JWT secret:** similarly, set a real random `JWT_SECRET` (32+ characters — e.g.
`openssl rand -base64 48`) on Render. The app also refuses to start under `prod` with the
default secret in place. Tokens expire after 12 hours by default (`JWT_EXPIRATION_MS`).

**Password requirements:** new accounts created via `POST /api/users` need at least 8
characters, with at least one letter and one number.

**1. Log in to get a token:**
```bash
curl -X POST https://your-backend.onrender.com/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "<your password>"}'
```
Returns `{"token": "...", "username": "admin", "role": "ADMIN"}`.

**2. Use the token on subsequent calls** (`Authorization: Bearer <token>`):
```bash
# Approve a citizen report
curl -X POST https://your-backend.onrender.com/api/localities/reports/3/approve \
  -H "Authorization: Bearer <token>"

# Add an editor (admin only)
curl -X POST https://your-backend.onrender.com/api/users \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"username": "priya", "password": "<their password>", "role": "EDITOR"}'
```

Tokens expire after 12 hours — log in again to get a fresh one; there's no refresh-token flow
yet, which is fine for occasional admin use but worth adding if this sees frequent editing.

There's no login screen in the frontend yet — it's a read-only public site by design. Admin/editor
actions go through the API directly (curl, Postman, etc.) until/unless a login UI gets added.

### Audit log

Every create/update/delete on localities, report approvals, and user account create/delete are
recorded automatically (an AOP aspect wraps the `@Audited`-annotated methods -- see
`backend/src/main/java/org/punewatertracker/audit/`) -- who did it (from the auth token),
what, when, and whether it succeeded. View it (admin only):

```bash
curl https://your-backend.onrender.com/api/audit-log -H "Authorization: Bearer <admin-token>"
```

To audit a new method later, just add `@Audited("SOME_ACTION_NAME")` to it -- no logging code
needed in the method itself.

## Running the frontend

Requires Node 18+.

```bash
cd frontend
npm install
npm run dev
```

Opens on `http://localhost:5173` and talks to the backend at `http://localhost:8080/api`
by default — copy `.env.example` to `.env` to point it elsewhere.

## Analytics (Google Analytics 4)

Disabled unless configured — with `VITE_GA_MEASUREMENT_ID` unset, `analytics.js` does nothing
at all, so local dev never sends real traffic to Google.

**Setup:**
1. Create a GA4 property at [analytics.google.com](https://analytics.google.com) — Admin →
   Create Property.
2. Under **Data Streams**, add a Web stream for your deployed URL and copy its **Measurement
   ID** (looks like `G-XXXXXXXXXX`).
3. Set `VITE_GA_MEASUREMENT_ID` on Vercel (Settings → Environment Variables) and redeploy —
   Vite bakes env vars in at *build* time, same as `VITE_API_BASE`, so an existing deployment
   won't pick this up without a fresh build.

This site has no client-side routing (all state changes happen without changing the URL), so
there's only ever one meaningful page view to track — no route-change tracking setup needed,
unlike most React + GA integration guides you'll find, which assume React Router.

## About the seed data

`LocalityDataInitializer` seeds ~39 localities compiled from PMC's 2026-27 budget announcement,
recent news coverage of merged-village tanker dependency, RTI data on tanker spending, and a
property-buyer water guide -- full citations are in each row's `sourceName`/`sourceUrl`. A few
long-established core areas (Kothrud, Aundh, etc.) are included as a general baseline without a
specific dated 2026 source, flagged as such in their notes -- treat those as lower-confidence
and worth a local check before relying on them. It only seeds if the table is empty, so it's
safe on every restart against a persistent database and won't duplicate rows.

This is a **starting point, not ground truth**. Update entries as PMC publishes new information,
as you verify things locally, or by reviewing citizen reports through the `/reports` endpoints.

**On the `ward` field:** PMC's 41 wards are numbered (1-41), not named -- there's no public,
fetchable mapping of "this locality is in ward N," since that requires GIS boundary data, not
just a locality's name or coordinates. Some entries have an informal area name in `ward` as a
loose hint; most are left `null` rather than guessing. If you want it accurate, PMC's own ward
locator (search "find my ward pmc.gov.in") is the reliable source per-address -- worth filling
in over time via the `PUT /api/localities/{id}` endpoint rather than trying to bulk-populate it.

## Ward boundaries

Resolves a gap flagged earlier in this project: PMC's 41 electoral wards are numbered, not
named, and redraw every election, with no boundary data published anywhere findable. PMC's
**15 administrative wards** (the ones tied to actual ward offices citizens go to) are more
stable and *do* have real boundary data available, hand-traced from official PMC ward PDFs.

**One manual step required** -- this isn't bundled in the repo:
1. Download [`pune-admin-wards.geojson`](https://github.com/datameet/Pune_wards/blob/master/GeoData/pune-admin-wards.geojson)
   (MIT / CC-BY-SA-2.5-India, via datameet/Pune_wards -- click "Raw" or "Download raw file").
2. Place it at `backend/src/main/resources/static/pune-admin-wards.geojson`.

That one file serves two purposes: `WardBoundaryService` parses it at startup for
point-in-polygon lookup (pure Java ray-casting, no geo library), and Spring Boot's static
resource handling serves the same file directly at `GET /pune-admin-wards.geojson` for the
frontend to draw as a map overlay (the "Ward boundaries" checkbox, top-right of the map).

Without the file present, everything degrades gracefully -- `WardBoundaryService` logs a
one-line notice at startup and all lookups return null, rather than failing to start.

**Endpoints:**
```bash
# Public: which ward is this point in
curl "https://your-backend.onrender.com/api/wards/lookup?lat=18.55&lng=73.85"

# Admin/editor: recompute + persist one locality's ward from its actual coordinates.
# Deliberately per-locality and manually triggered, not a silent bulk overwrite -- an admin
# reviews before an existing (possibly hand-verified) ward value gets replaced.
curl -X PUT https://your-backend.onrender.com/api/wards/5/recompute -H "Authorization: Bearer <token>"
```

## Source-link health checking

Every locality's `sourceUrl` gets checked for liveness -- news articles get paywalled, moved,
or taken down, and a curated dataset like this one needs a way to notice. Runs automatically
every Monday 03:00, or trigger it on demand (admin/editor):

```bash
curl -X POST https://your-backend.onrender.com/api/localities/check-source-links \
  -H "Authorization: Bearer <token>"
```

Returns immediately (`202 Accepted`) -- the checks themselves run concurrently on a dedicated
4-8 thread pool (`TaskExecutorConfig`), separate from request-handling threads, so a slow batch
of checks can't block the API. Each `Locality` gets `sourceUrlHealthy` (true/false/null =
never checked) and `sourceUrlLastChecked` set once its check completes -- both already show up
in the normal `GET /api/localities` response, so the frontend could surface a "citation may be
stale" indicator without any further backend work.

## MCP server (AI assistant access)

Exposes read-only locality data as tools any MCP-compatible AI assistant (Claude Desktop, etc.)
can call directly -- someone could ask "what's the water status in Wagholi?" through their own
assistant without ever visiting this site. Deliberately read-only: mirrors only what's already
public via `GET /api/localities`, never the admin/write operations.

**This is the least-verified feature in this codebase.** The MCP Java SDK's API has shifted
across recent versions, and I could not compile-check this locally. Specifically at risk:
`HttpServletStreamableServerTransportProvider`'s constructor signature in
`McpServerConfig.java` -- if `mvn compile` fails, start there, and check the current API
against https://github.com/modelcontextprotocol/java-sdk for whichever SDK version actually
resolves. **Run `mvn compile` locally before pushing this one**, more than any other feature
in this project.

**Tools exposed:**
- `get_locality_status(name)` -- look up a locality by name
- `list_localities_by_status(status)` -- list localities matching MUNICIPAL / TANKER_DEPENDENT / MIXED / PIPELINE_IN_PROGRESS

**Endpoint:** `https://your-backend.onrender.com/mcp` (Streamable HTTP transport). Point an MCP
client (e.g. Claude Desktop's config, or any MCP-compatible tool) at that URL to try it.

## Caching

`GET /api/localities` and `GET /api/localities/{id}` are cached (Caffeine, in-memory, 30s TTL)
-- not because the query is slow (it isn't, at this data size), but because it's hit on every
frontend page load against data that changes rarely, and every hit costs a round-trip to
Aiven's free-tier connection pool for no reason.

**Deliberately Caffeine-only, no Redis.** A second (Redis) tier only earns its complexity once
there's more than one running instance needing cross-instance cache invalidation -- Render's
free tier runs a single instance, so that problem doesn't exist yet. Revisit this if that
changes.

**Eviction is precise, not blanket:** writes that actually change what a cached read returns
(`createVerified`, `approveReport`, `update`, `delete`) evict the cache; `createUnverifiedReport`
deliberately doesn't, since an unverified report never shows up in the cached results anyway.

**One known gap:** `SourceLinkHealthChecker` writes directly via `LocalityRepository`, bypassing
`LocalityService` entirely -- so its weekly link-health updates don't trigger cache eviction.
In practice this just means up to a 30s delay before `sourceUrlHealthy` changes show up in the
API, which is fine for a background job that isn't time-critical, but worth knowing rather than
discovering by surprise.

## Issue classification (gRPC)

Citizens don't need to know PMC's status vocabulary to report a problem -- instead of picking
from `MUNICIPAL`/`TANKER_DEPENDENT`/etc., they can just describe what's wrong
(`"No water since yesterday"`, `"Pipeline burst near ABC Chowk"`, `"Water smells bad"`), and
the backend suggests a category via an internal gRPC call.

**Why gRPC here specifically:** this call is service-to-service (your Spring Boot backend
calling a classifier), never browser-facing -- the citizen's browser only ever talks REST/JSON
to your existing API, same as before. That's exactly the kind of internal, strongly-typed
contract gRPC is built for, versus a public API where REST/JSON remains the right choice.

**How it's deployed:** the classification server runs *in-process*, in the same JVM as the
REST API, on its own port (`9090` by default) -- not a separate Render service. Real `.proto`
codegen, a real gRPC server (`IssueClassificationServiceImpl`) and client
(`IssueClassificationClient`), just not yet split into an independently deployed service. If
this ever needs to scale independently, that's a network-config change (point the client at a
different host), not a rewrite.

**Submit a report with free text instead of a status:**
```bash
curl -X POST https://your-backend.onrender.com/api/localities/reports \
  -H "Content-Type: application/json" \
  -d '{"name": "Wagholi", "latitude": 18.58, "longitude": 73.98, "status": "TANKER_DEPENDENT", "citizenDescription": "Pipeline burst near ABC Chowk this morning"}'
```
The response includes a `reportedIssueCategory` the classifier suggested (`PIPELINE_BREAK` for
that example) alongside the `status` you still provide -- `reportedIssueCategory` is a triage
hint for whoever reviews the report, separate from `status`, which remains what actually drives
the public map. If the citizen (or you) already sets `reportedIssueCategory` explicitly, the
classifier is skipped and that value is left alone.

**The default classifier is keyword-based** (`IssueClassificationServiceImpl`) -- free,
deterministic, works immediately with no API key. To swap in a real LLM call instead, replace
that class's body with an HTTP call to your provider of choice and map its response onto
`IssueCategory`; everything else (the `.proto` contract, the client, the graceful fallback if
classification fails) stays the same.

**A note on the Maven build:** the protobuf codegen plugin (`protobuf-maven-plugin`) is the
part most likely to need a version tweak on your machine -- it downloads OS-specific `protoc`
and `protoc-gen-grpc-java` binaries on first build, and I can't verify this compiles from this
sandbox (no Maven Central access here). `mvn compile` locally is the real test; if it fails on
the proto step specifically (not elsewhere), that's the plugin/binary versions to look at first.

## Chaos engineering (Chaos Monkey for Spring Boot)

**Read this before turning it on.** Chaos Monkey injects real faults (latency, exceptions,
even killing the JVM) into live requests. It's double-gated to make accidental activation
hard: it only does anything if **both** the `chaos-monkey` Spring profile is active **and**
`chaos.monkey.enabled=true` -- neither `dev` nor `prod` alone ever triggers it. Even so:
**don't run this against your real deployed Render/Aiven instance until you've tried it
locally first and understand what each assault actually does.** Latency assaults are
relatively harmless; exception and app-killer assaults will break real requests or crash the
process, on purpose.

**Try it locally:**
```bash
mvn spring-boot:run -Dspring-boot.run.profiles=dev,chaos-monkey
```
Ships active with latency-only assaults (1-3s delay, ~20% of watched calls) -- everything
else (exceptions, app-killing, memory pressure) is off by default in
`application-chaos-monkey.properties`; turn those on live instead of baking them into a
config file, via the actuator endpoint (admin token required):

```bash
# Check current status
curl https://localhost:8080/actuator/chaosmonkey/status -H "Authorization: Bearer <admin-token>"

# Turn exception assaults on (this will start failing real requests)
curl -X POST https://localhost:8080/actuator/chaosmonkey/assaults \
  -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" \
  -d '{"exceptionsActive": true}'

# Kill switch -- disables Chaos Monkey entirely without restarting the app
curl -X POST https://localhost:8080/actuator/chaosmonkey/disable -H "Authorization: Bearer <admin-token>"
```

If you do eventually want to run this against a real (non-production-critical) deployed
instance to test how the system behaves under real network conditions, start with latency
only, at a low `level` (rare), and keep the disable command ready in another tab.

## Testing

```bash
cd backend
mvn test
```

- `LocalityServiceTest`, `JwtServiceTest`, `AdminBootstrapTest` -- pure Mockito unit tests, no
  Spring context, run instantly.
- `RabbitMQIntegrationTest` -- spins up a real RabbitMQ broker via **Testcontainers** and
  proves `ReportEventPublisher` actually delivers a message onto it, rather than just mocking
  `RabbitTemplate` and checking mocks agree with each other. **Requires Docker running
  locally** (or in CI) -- `mvn test` will fail this specific test if Docker isn't available,
  the other tests are unaffected.

## Deploying with a hosted MySQL database (Aiven)

The `prod` Spring profile (`application-prod.properties`) points at MySQL via env vars.
[Aiven](https://aiven.io) has a genuinely free MySQL plan (1GB, no credit card, no time limit)
that works directly with it -- and unlike AWS RDS, there's no VPC/security-group setup needed
since it's open to the internet by default (secured by SSL + password, not IP allowlisting).

**1. Create the service**
Aiven dashboard → Create service → MySQL → Free plan → pick any region → name it → Create.
Takes a couple minutes to provision.

**2. Grab your connection details**
From the service overview page: **Host**, **Port**, **User** (usually `avnadmin`),
**Password**, and the **default database** name.

**3. Set env vars on Render** (service → Environment tab):
```
SPRING_PROFILES_ACTIVE=prod
DB_HOST=<your-aiven-host>
DB_PORT=<your-aiven-port>
DB_NAME=<your-aiven-database-name>
DB_USERNAME=avnadmin
DB_PASSWORD=<your-aiven-password>
ADMIN_USERNAME=<a real admin username>
ADMIN_PASSWORD=<a real admin password>
```
Save → Render redeploys → `AdminBootstrap` creates your admin account and
`LocalityDataInitializer` seeds the localities, both against Aiven this time, both persisting
across future redeploys since `ddl-auto=update` no longer wipes the schema.

Note the free Aiven plan's 1GB storage cap -- comfortably enough for this project's data, but
worth knowing if you plan to grow it substantially.

## Logging (Loki)

Console logging always works (visible in Render's own Logs tab), but it's basic and ephemeral
-- no search across deploys, limited retention. Shipping to **Grafana Loki** fixes that, and
if you already set up Grafana Cloud for traces/metrics above, it's the **same account, same
free tier** -- Loki is just Grafana Cloud's log storage, sitting next to Tempo and Prometheus.

Uses [Loki4j](https://loki4j.github.io/loki-logback-appender/) (`com.github.loki4j:loki-logback-appender`),
a stable, actively maintained Logback appender built specifically for this. **Disabled by
default** -- with `LOKI_URL` unset, logging behaves exactly as it did before this was added.

**Setup:**
1. In your Grafana Cloud stack: **Connections → Add new connection** → search for a Loki/logs
   connection option -- it'll generate the push URL and credentials for you (an instance ID
   and an API token) rather than you constructing them by hand.
2. Set on Render: `LOKI_URL` (the push endpoint, ending in `/loki/api/v1/push`), `LOKI_USERNAME`
   (your instance ID), `LOKI_PASSWORD` (the API token).
3. Redeploy. Logs should start appearing in Grafana Cloud's **Explore** tab under the Loki
   data source within a minute or two.

**One thing to flag honestly**: the `<auth>` block in `logback-spring.xml` is my best-effort
construction, not independently verified against Loki4j's exact current XML schema (unlike
the core dependency/appender class, which is confirmed against Loki4j's own docs). If logs
aren't showing up after setup, that block is the first thing to check against
https://loki4j.github.io/loki-logback-appender/ 's current auth configuration examples.

## Observability (OpenTelemetry + Grafana Cloud)

Traces and metrics export via OTLP -- disabled in `dev` (no local collector, would just spam
connection-refused logs), configured for any OTLP backend in `prod`. Uses Spring Boot 3's
native Micrometer Tracing + OTel bridge, not the Java agent -- it's a Maven dependency, no
extra JVM flags needed on Render's Docker setup. `service.name` is picked up automatically
from `spring.application.name` (`pmc-water-tracker`) -- nothing extra to configure there.

**Grafana Cloud's free tier** is the easiest path to an actual dashboard with no infra to run
yourself:

1. Sign up at grafana.com -- a free stack is created automatically.
2. In your stack, go to **Connections → Add new connection** → search **OpenTelemetry** →
   follow their OTLP setup flow. It generates the exact endpoint URLs and a pre-encoded
   `Authorization` header for you (Grafana Cloud uses Basic auth: `base64(instanceID:apiToken)`) --
   copy both rather than constructing them by hand.
3. Set on Render (backend service → Environment tab):
   ```
   OTLP_TRACES_ENDPOINT=<traces endpoint from Grafana's setup flow>
   OTLP_METRICS_ENDPOINT=<metrics endpoint from Grafana's setup flow>
   OTLP_AUTH_HEADER=<the Authorization header value Grafana generated, e.g. "Basic xxxxx">
   ```
4. Redeploy. Traffic against the API now generates traces/metrics flowing into Grafana Cloud.
5. View it: your stack's hosted Grafana UI (`<your-stack>.grafana.net`) → **Explore** → pick
   the Tempo (traces) or Prometheus (metrics) data source Grafana Cloud auto-provisioned --
   data should show up within a minute or two of hitting the API. Build a proper dashboard from
   there once you know which metrics/traces actually matter to you.

Check Grafana Cloud's current free-tier limits (data volume, retention) on their pricing page --
worth knowing before you lean on it heavily, though it's generous enough for a project this size.

`management.endpoints.web.exposure.include` also exposes `/actuator/health`, `/actuator/info`,
and `/actuator/metrics` directly if you want to check something without leaving Render.

## Optional: async report notifications (RabbitMQ)

When a citizen submits a report (`POST /api/localities/reports`), the app can publish a
`report.submitted` event -- useful for notifying admins (email/Slack/etc.) without slowing
down or risking the citizen's actual request. **Disabled by default** (`RABBITMQ_ENABLED=false`)
-- with no broker configured, nothing changes; report submission works exactly as before.

**To enable it:**
1. Get a broker. [CloudAMQP](https://www.cloudamqp.com) has a genuine free tier ("Little
   Lemur" -- small, but free, no card required).
2. Set env vars: `RABBITMQ_ENABLED=true`, `RABBITMQ_HOST`, `RABBITMQ_PORT` (usually `5671` for
   TLS), `RABBITMQ_USERNAME`, `RABBITMQ_PASSWORD`, `RABBITMQ_VHOST` (CloudAMQP's vhost is
   usually the same as the username), `RABBITMQ_SSL_ENABLED=true`.
3. For email notifications, also set: `BREVO_API_KEY`, `MAIL_FROM_ADDRESS`, and
   `ADMIN_NOTIFICATION_EMAIL`. Uses [Brevo](https://www.brevo.com)'s HTTP API (free tier: 300
   emails/day) instead of SMTP -- **Render's free tier blocks all outbound SMTP traffic**
   (ports 25/465/587), so Gmail SMTP or any other SMTP provider cannot work there regardless
   of credentials; Brevo's API goes over HTTPS like everything else, so it isn't affected.
   Sign up free, verify a sender email address (that's your `MAIL_FROM_ADDRESS`), generate an
   API key under **SMTP & API** settings (that's `BREVO_API_KEY`). `ADMIN_NOTIFICATION_EMAIL`
   is wherever you want reports sent *to* -- can be any address, doesn't need verifying.
4. Redeploy. `RabbitMQConfig` creates the exchange/queue, `ReportEventPublisher` starts
   publishing, and `ReportNotificationListener` consumes and emails
   `ADMIN_NOTIFICATION_EMAIL` via Brevo with the report details and a direct approve link. If
   mail env vars are missing, it logs and skips the email rather than failing -- the report
   itself always saves successfully either way.

## Next steps worth considering

- A login screen + admin UI in the frontend for reviewing pending citizen reports and managing
  localities, rather than calling the API directly.
