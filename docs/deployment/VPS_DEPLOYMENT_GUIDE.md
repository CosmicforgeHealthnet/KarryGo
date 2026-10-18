# KarryGo VPS Deployment Guide

A plain-language, step-by-step guide to running every KarryGo backend service on
the VPS with Docker, and deploying changes automatically from GitHub.

You don't need to be a server expert. Every command can be copied and pasted,
and each step says **what you should see** so you know it worked.

> **Golden rules**
> 1. Look before you change: run the read-only inventory first.
> 2. Passwords never go into GitHub. They live in private files on the server.
> 3. Change one service at a time, and check it's healthy before the next.
> 4. This server is **shared** with other products (cfmeet, LiveKit, the
>    cosmicforge backend, admin-server). Only touch things named `karrygo`.

---

## Contents

- [A. The big picture](#a-the-big-picture)
- [B. Words you'll see](#b-words-youll-see)
- [C. Before you start (checklist)](#c-before-you-start-checklist)
- [D. Look before you touch (inventory)](#d-look-before-you-touch-inventory)
- [E. Close the open ports (do this first)](#e-close-the-open-ports-do-this-first)
- [F. Prepare the server](#f-prepare-the-server)
- [G. Settings files (env): how they get onto the server](#g-settings-files-env-how-they-get-onto-the-server)
- [H. Secrets that must match](#h-secrets-that-must-match)
- [I. GitHub setup](#i-github-setup)
- [J. Build the images for the first time](#j-build-the-images-for-the-first-time)
- [K. Adopt the services that already run](#k-adopt-the-services-that-already-run)
- [L. Add the new services](#l-add-the-new-services)
- [M. Tighten the front door (nginx)](#m-tighten-the-front-door-nginx)
- [N. Everyday: how a change reaches the server](#n-everyday-how-a-change-reaches-the-server)
- [O. Is it working? Health, logs, status](#o-is-it-working-health-logs-status)
- [P. Rolling back](#p-rolling-back)
- [Q. Changing a setting](#q-changing-a-setting)
- [Push notifications (Firebase)](#push-notifications-firebase)
- [R. Backups and restoring](#r-backups-and-restoring)
- [S. Monitoring](#s-monitoring)
- [T. Security checklist](#t-security-checklist)
- [U. Troubleshooting](#u-troubleshooting)
- [V. Pointing the mobile apps at the server](#v-pointing-the-mobile-apps-at-the-server)
- [W. Reference: every service at a glance](#w-reference-every-service-at-a-glance)

---

## A. The big picture

```
 Phone app ──HTTPS──▶ api-gateway.staging.karryam.com
                            │
                       ┌────▼─────┐   the server's "front door";
                       │  nginx   │   blocks private pages, adds HTTPS
                       └────┬─────┘
                            │ (inside the server only)
                     ┌──────▼───────┐
                     │ api-gateway  │  forwards each request by its path
                     └──┬───┬───┬───┘
        ┌───────────────┘   │   └────────────────┐
   customer, taxi,     payment-wallet,      vendor, fleet ...
   hauling, dispatch   notification ...
        │                   │                    │
   each service has its OWN small database (and Redis if it needs one)
```

**How a code change gets there (CI/CD):**

```
you push to "staging" ──▶ GitHub tests the changed services
                     ──▶ GitHub builds an image per service, labelled with the commit
                     ──▶ GitHub stores it in GHCR (GitHub's image warehouse)
                     ──▶ GitHub logs into the server as "deploy"
                     ──▶ the server downloads the image, prepares the database,
                         starts the new version, waits until it is healthy
                     ──▶ if it is NOT healthy, the old version is put back automatically
```

**Where things live on the server:**

| Path | What it is |
|---|---|
| `/opt/karrygo/platform/` | The deploy files from the repo (`deploy/platform/`). GitHub refreshes it on each deploy. |
| `/opt/karrygo/<service-home>/<name>.env` | A service's private settings (mode 600). Notification and fleet use `notification.service.env` / `fleet.service.env`. |
| `/opt/karrygo/<service-home>/db.env` | Database/Redis passwords for new services (mode 600). |
| `/opt/karrygo/<service-home>/release.env` | Which image version is deployed now. |
| `/opt/karrygo/<service-home>/release.previous.env` | The version before it (for rollback). |
| `/opt/karrygo/backups/` | Nightly backups. |

**You do not need a separate domain for each service.** One address,
`api-gateway.staging.karryam.com`, serves the whole app; nginx and the gateway
send each request to the right service by its path. `vendor-api.staging.karryam.com`
and `fleet-api.staging.karryam.com` already exist for the two web consoles.

**You do not need Docker Hub.** GitHub builds the images and GHCR stores them.

---

## B. Words you'll see

| Word | Plain meaning |
|---|---|
| **Image** | A sealed box holding a program and everything it needs. |
| **Container** | A running copy of an image. |
| **Compose file** | A written list of containers and their settings; one command starts exactly that. Ours are in `deploy/platform/compose/`. |
| **Volume** | A folder Docker keeps on disk for a container (databases store data here). Survives container replacement. |
| **GHCR** | GitHub Container Registry: GitHub's image warehouse. |
| **GitHub Actions** | GitHub's robot that builds, tests and deploys. |
| **Secret (GitHub)** | A value GitHub stores encrypted and never shows again. |
| **Env file** | A private text file of `NAME=value` settings for one service. |
| **Migration** | A file that creates or changes database tables. |
| **Health check** | Docker asks the service "are you ready?" (`/ready`) every 10 seconds. |
| **Rollback** | Going back to the previous version. |
| **SSH** | A secure way to log in to the server from your Mac. |
| **nginx** | The web server in front of everything; handles HTTPS. |
| **Port** | A numbered "door" on the server. Only 22 (SSH), 80 and 443 (web) should face the internet. |

---

## C. Before you start (checklist)

**Must have**

- [ ] Admin access to the GitHub repo `CosmicforgeHealthnet/KarryGo`.
- [ ] You can log in to the server: `bash scripts/ssh-vps.sh` (run
      `bash scripts/ssh-vps.sh setup-key` once first if it asks for a password).
- [ ] You've asked your team whether anything uses the raw addresses
      `http://173.249.30.55:4100`, `:4101` or `:4103` (step E closes them).
- [ ] A quiet time for a few seconds of restart per service (step K).
- [ ] A **backup passphrase** you made up and saved in a password manager
      (step R). Never send it to anyone.

**Already on the server, just confirm they're real (not test values)**

- [ ] Paystack secret key in payment-wallet's settings (top-ups, payments, withdrawals).
- [ ] Firebase credentials in notification and media settings (push, file storage).
- [ ] Email (SMTP) account in notification's settings (login codes by email).
      Note: the platform has **no SMS provider**, so phone login codes can't be texted yet.

**Nice to have**

- [ ] A Google Cloud "Routes API" key for taxi and hauling (real road routes on
      the map instead of straight lines).
- [ ] An off-server place for backups (for example Backblaze B2) and `rclone`.
- [ ] A free uptime monitor (for example UptimeRobot).

---

## D. Look before you touch (inventory)

This only **reads** information and changes nothing. Do it before every big change.

The first time, before GitHub has ever deployed, copy the deploy folder to the
server from your Mac (later, every GitHub deploy refreshes it for you):

```bash
# on your Mac, in the repo folder
scp -r deploy/platform root@173.249.30.55:/opt/karrygo/
ssh root@173.249.30.55 'chmod +x /opt/karrygo/platform/bin/*.sh'
```

Then run the inventory:

```bash
bash scripts/ssh-vps.sh            # log in from your Mac
bash /opt/karrygo/platform/bin/inventory.sh | tee ~/inventory-$(date +%F).txt
```

**What you should see:** sections for SERVER, DOCKER, CONTAINERS, VOLUMES,
`/opt/karrygo`, LISTENING PORTS, NGINX, FIREWALL and SWAP. Passwords in compose
files show as `***hidden***`; private settings files are never opened.

**What it showed on 2026-10-17** (so you know what "normal" looks like):

- Ubuntu 24.04, 6 CPUs, 11.7 GB memory (8.4 GB free), 109 GB free disk, **no swap**.
- 9 KarryGo services running: api-gateway, notification, dispatch, media (+ ClamAV
  scanner), payment-wallet, verification, vendor (API + worker), fleet (API + worker).
- Only gateway, media and vendor had compose files; the rest were **started by hand**.
- Each service has its own database container (`karrygo-*-db`, `karrygo-*-postgres`).
- **Ports 4100, 4101 and 4103 were open to the internet** (fixed in step E).
- Dispatch stored uploaded photos **inside its container** in `/tmp` (handled in step K).
- The modern `docker compose` command was missing (fixed in step F).

---

## E. Close the open ports (do this first)

**Why:** Docker writes its own firewall rules that run *before* Ubuntu's firewall.
So ports 4100 (gateway), 4101 (media) and 4103 (dispatch) were reachable by anyone
over plain HTTP, skipping nginx's padlock and its "block private pages" rules.

**How:** this adds a block that Docker respects. Nothing restarts. nginx keeps
working because it talks to these ports from *inside* the server.

```bash
sudo bash /opt/karrygo/platform/bin/close-public-ports.sh
sudo bash /opt/karrygo/platform/bin/close-public-ports.sh --check
```

**What you should see:**
`Blocked from the internet on eth0: 4100 4101 4103` and
`Will be re-applied automatically after every reboot`.

**Prove it from your Mac** (not the server):

```bash
nc -z -w 4 173.249.30.55 4100 && echo "STILL OPEN" || echo "closed - good"
curl -s -o /dev/null -w '%{http_code}\n' https://api-gateway.staging.karryam.com/health   # 200
```

**To undo** (only if something legitimate broke):
`sudo bash /opt/karrygo/platform/bin/close-public-ports.sh --undo`.

---

## F. Prepare the server

### F1. Create the key GitHub will use to log in

On your **Mac**:

```bash
ssh-keygen -t ed25519 -f ~/.ssh/karrygo_github_deploy -N "" -C "github-actions-deploy"
cat ~/.ssh/karrygo_github_deploy.pub     # the PUBLIC half: goes on the server (next step)
```

The **private** half (`~/.ssh/karrygo_github_deploy`, no `.pub`) goes into GitHub
in step I. Never paste it anywhere else.

### F2. Run the setup script

```bash
bash scripts/ssh-vps.sh
sudo bash /opt/karrygo/platform/bin/server-setup.sh "ssh-ed25519 AAAA...paste the .pub line..."
```

It's safe to run again; each step skips what's already done. It:

| Step | Why |
|---|---|
| Installs the `docker compose` plugin | The GitHub workflows use it; it was missing. |
| Adds 4 GB swap | Emergency memory: without it, a memory spike can crash the whole shared server. |
| Installs fail2ban + automatic security updates | Blocks password guessing; keeps Ubuntu patched. |
| Creates the `deploy` user with your key | GitHub logs in as this user instead of `root`. |
| Gives `deploy` ownership of `/opt/karrygo` | So it can read the private settings files (they stay owner-only). |

It deliberately does **not** change Docker's own configuration, because that
would restart every container on the server, including the other products.

**What you should see:** `== done`. Then test from your Mac:

```bash
ssh -i ~/.ssh/karrygo_github_deploy deploy@173.249.30.55 'docker ps --format "{{.Names}}" | head -3'
```

(It should list container names without asking for a password.) Note that
being in the `docker` group gives `deploy` near-root power on the server, which
is why its key lives only in GitHub's encrypted secrets.

### F3. Turn off password logins (recommended)

Only after `bash scripts/ssh-vps.sh` logs you in **without** a password:

```bash
sudo sed -i 's/^#\?PasswordAuthentication .*/PasswordAuthentication no/' /etc/ssh/sshd_config
sudo sshd -t && sudo systemctl reload ssh
```

Keep your current session open and test a new login in a second terminal before closing it.

---

## G. Settings files (env): how they get onto the server

Each service reads its settings from one private file on the server. GitHub
never sees them. The rules:

- One file per service, for example `/opt/karrygo/customer-service/customer.env`.
- **Mode 600** (owner only). Deploys refuse to run if it's missing or readable by others.
- Templates with names and explanations (no values) are in `deploy/platform/env/`.
- `.gitignore` blocks real settings files from ever being committed.

### G1. Creating one

**Option 1: type it on the server**

```bash
bash scripts/ssh-vps.sh
sudo -u deploy mkdir -p /opt/karrygo/customer-service
sudo -u deploy nano /opt/karrygo/customer-service/customer.env    # paste, Ctrl-O to save, Ctrl-X to exit
sudo -u deploy chmod 600 /opt/karrygo/customer-service/customer.env
```

**Option 2: fill it on your Mac, then send it**

```bash
scp customer.env deploy@173.249.30.55:/opt/karrygo/customer-service/   # uses the deploy key or your own login
ssh deploy@173.249.30.55 chmod 600 /opt/karrygo/customer-service/customer.env
rm customer.env                                                        # do not keep copies lying around
```

### G2. Filling one in

| Placeholder in the template | What to put |
|---|---|
| `GENERATE` | A new random value. Run `openssl rand -hex 32` and paste the output. |
| `MUST MATCH ...` | The exact value from the named setting of the named service (see section H). |
| `<db.env POSTGRES_PASSWORD>` | The password you put in that service's `db.env`. |
| `YOUR ...` | Your own account value (for example a Google key). |

Leave every "debug" switch (`*_DEBUG_OTP`, `*_OTP_DEBUG`, `DISPATCH_RIDER_DEBUG_OTP`)
set to `false`. If one were `true`, the API would send people's login codes
back in its responses.

### G3. Capturing the settings of a hand-started service

Some running services had their settings typed into the start command, so they
exist **only inside the running container**. This writes them to a file
without ever showing the values:

```bash
cd /opt/karrygo/platform
bin/export-container-env.sh karrygo-payment-wallet-service /opt/karrygo/payment-wallet-service/wallet.env
```

**What you should see:** `wrote 16 settings to ... (mode 600). Names: APP_ENV HTTP_ADDR ...`.
It refuses to overwrite an existing file.

---

## H. Secrets that must match

Services trust each other with shared secrets. If two values that must match
don't, requests fail and the service usually just logs "unauthorized". This
table lists every pair to get right.

**Login-token secrets** (one app's login is checked by several services)

| Secret | Set in | Must be identical in |
|---|---|---|
| Customer login | customer `CUSTOMER_ACCESS_TOKEN_SECRET` | payment-wallet `PAYMENT_WALLET_CUSTOMER_ACCESS_TOKEN_SECRET` (already set: **copy this value**), dispatch `DISPATCH_CUSTOMER_TOKEN_SECRET`, hauling `HAULING_CUSTOMER_TOKEN_SECRET`, taxi `TAXI_CUSTOMER_TOKEN_SECRET`, support `SUPPORT_DISPUTE_CUSTOMER_ACCESS_TOKEN_SECRET`, referral `REFERRAL_CUSTOMER_TOKEN_SECRET` |
| Truck driver login | hauling `HAULING_PROVIDER_TOKEN_SECRET` | support `SUPPORT_DISPUTE_HAULING_PROVIDER_TOKEN_SECRET`, payment-wallet `PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS` (the `hauling=` entry), referral `REFERRAL_PROVIDER_TOKEN_SECRETS` (`hauling=`) |
| Taxi driver login | taxi `TAXI_PROVIDER_TOKEN_SECRET` | support `SUPPORT_DISPUTE_TAXI_PROVIDER_TOKEN_SECRET`, payment-wallet (`taxi=` entry), referral (`taxi=`) |
| Dispatch rider login | dispatch `DISPATCH_RIDER_ACCESS_TOKEN_SECRET` | support `SUPPORT_DISPUTE_DISPATCH_PROVIDER_TOKEN_SECRET`, payment-wallet (`dispatch=` entry), referral (`dispatch=`) |

> Dispatch currently has **no** `DISPATCH_CUSTOMER_TOKEN_SECRET`, so it falls back
> to a development default. Add the real customer secret to `dispatch.env` and
> apply it (section Q) when customer-service goes live.

**Service-to-service secrets** (the caller's secret must appear in the receiver's list)

| Caller sets | Receiver's list must contain |
|---|---|
| customer `CUSTOMER_NOTIFICATION_SECRET` | notification `NOTIFICATION_SERVICE_SECRETS`: `customer-service=<same>` |
| hauling `HAULING_NOTIFICATION_SECRET` | `NOTIFICATION_SERVICE_SECRETS`: `driver-hauling-service=<same>` |
| taxi `TAXI_NOTIFICATION_SECRET` | `NOTIFICATION_SERVICE_SECRETS`: `driver-taxi-service=<same>` |
| support `SUPPORT_DISPUTE_NOTIFICATION_SECRET` | `NOTIFICATION_SERVICE_SECRETS`: `support-dispute-service=<same>` |
| hauling `HAULING_PAYMENT_SECRET` | payment-wallet `PAYMENT_WALLET_SERVICE_SECRETS`: `driver-hauling-service=<same>` |
| taxi `TAXI_PAYMENT_SECRET` | `PAYMENT_WALLET_SERVICE_SECRETS`: `driver-taxi-service=<same>` |
| support `SUPPORT_DISPUTE_PAYMENT_SECRET` | `PAYMENT_WALLET_SERVICE_SECRETS`: `support-dispute-service=<same>` |
| customer `CUSTOMER_MEDIA_SERVICE_TOKEN` | media `MEDIA_FILE_SERVICE_TOKENS`: `customer-service=<same>` |
| hauling `HAULING_MEDIA_TOKEN` | `MEDIA_FILE_SERVICE_TOKENS`: `driver-hauling-service=<same>` |
| taxi `TAXI_MEDIA_TOKEN` | `MEDIA_FILE_SERVICE_TOKENS`: `driver-taxi-service=<same>` |
| support `SUPPORT_DISPUTE_CUSTOMER_SERVICE_SECRET` | customer `CUSTOMER_SERVICE_SECRETS`: `support-dispute-service=<same>` |
| support `SUPPORT_DISPUTE_HAULING_SERVICE_SECRET` | hauling `HAULING_SERVICE_SECRETS`: `support-dispute-service=<same>` |
| support `SUPPORT_DISPUTE_TAXI_SERVICE_SECRET` | taxi `TAXI_SERVICE_SECRETS`: `support-dispute-service=<same>` |
| support `SUPPORT_DISPUTE_DISPATCH_SERVICE_SECRET` | dispatch `DISPATCH_SERVICE_SECRETS`: `support-dispute-service=<same>` |

Format of a list: `name=secret,name=secret` with no spaces. After editing a
receiver's file, apply it (section Q).

---

## I. GitHub setup

Do this in the GitHub website for `CosmicforgeHealthnet/KarryGo`.

1. **Settings → Secrets and variables → Actions → New repository secret.** Add or update:

   | Name | Value |
   |---|---|
   | `VPS_HOST` | `173.249.30.55` |
   | `VPS_USER` | `deploy` |
   | `VPS_SSH_KEY` | the whole **private** key: on your Mac, `cat ~/.ssh/karrygo_github_deploy`, including the `-----BEGIN` and `-----END` lines |

   Set them even if they exist: GitHub hides the old values, which may point at
   another server or user.

2. **Settings → Actions → General → Workflow permissions:** choose **"Read and
   write permissions"** and save. (Without it GitHub can build images but not store them.)

3. *(Optional)* **Settings → Environments → New environment** named `staging`.
   Add yourself under "Required reviewers" if you want to approve each deploy
   with a click.

4. *(Optional)* **Settings → Secrets and variables → Actions → Variables**: add
   `PUBLIC_BASE_URL` if the public address ever changes (default
   `https://api-gateway.staging.karryam.com`).

No Docker Hub account and no extra GHCR token are needed: the workflow logs the
server into GHCR with a short-lived token for each deploy.

---

## J. Build the images for the first time

1. Merge the deployment changes into `staging` and push.
2. Open GitHub → **Actions → Deploy platform**. A run starts by itself (or click
   **Run workflow**, keep `all`).
3. Watch the jobs: **Work out what changed → Test and build (one per service) → Deploy**. A service whose tests fail is held back on its own; the others still build and deploy.

On this first run, **Deploy** skips every service with a note like:

```
customer-service has never been deployed by this pipeline, so it is skipped.
  /opt/karrygo/platform/bin/deploy-service.sh customer-service ghcr.io/cosmicforgehealthnet/customer-service:<commit>
```

That's expected and safe: the pipeline never takes over a running service on
its own. You do that once by hand in steps K and L, using the image names
printed here. Copy the `<commit>` value; you'll need it below.

**To let the server download images by hand** (needed for K and L only), log in once:

```bash
# on your Mac: GitHub → Settings → Developer settings → Personal access tokens (classic)
# → generate a token with ONLY "read:packages" ticked. Then on the server:
sudo -u deploy docker login ghcr.io -u <your-github-username>    # paste the token as the password
```

---

## K. Adopt the services that already run

"Adopting" means the hand-started containers get recreated from the compose
files, so their settings are written down and GitHub can update them from then
on. `adopt.sh` keeps each old container **stopped but saved** and puts it back
automatically if the new one fails. Databases are never touched.

Do **one service at a time**, in this order, and check the app still works between each.

All commands run on the server as `deploy`:

```bash
bash scripts/ssh-vps.sh
sudo -iu deploy
cd /opt/karrygo/platform
C=<commit from step J>
R=ghcr.io/cosmicforgehealthnet
```

### K1. Capture or rename each settings file

| Service | Command |
|---|---|
| payment-wallet | `mkdir -p /opt/karrygo/payment-wallet-service && bin/export-container-env.sh karrygo-payment-wallet-service /opt/karrygo/payment-wallet-service/wallet.env` |
| dispatch | `bin/export-container-env.sh karrygo-dispatch-delivery-service /opt/karrygo/dispatch-delivery-service/dispatch.env` |
| verification | `mv /opt/karrygo/verification-service/verification.env /opt/karrygo/verification-service/verification.env.orig && bin/export-container-env.sh karrygo-verification-staging /opt/karrygo/verification-service/verification.env` |
| notification | `bin/export-container-env.sh karrygo-notification-service /opt/karrygo/notification-service/notification.service.env` (a NEW file; never touch `notification.env`, the old container reads it) |
| fleet | `cp /opt/karrygo/fleet-partner-service/fleet.env /opt/karrygo/fleet-partner-service/fleet.service.env && chmod 600 /opt/karrygo/fleet-partner-service/fleet.service.env` (a NEW file; never touch `fleet.env`, the old container reads it) |
| api-gateway | `bin/export-container-env.sh karrygo-api-gateway /opt/karrygo/api-gateway/gateway.env` |
| media | `bin/export-container-env.sh karrygo-media-file-service /opt/karrygo/media-file-service/media.env` |

> **Never change a file an old container still reads.** The old container is
> the fallback if adoption fails; notification and fleet read their settings
> file through a mount, so their compose files use separate names
> (`*.service.env`). `adopt.sh` refuses to run if this would be violated.

Then fix two things in the exported files:

- **dispatch.env:** `AVAILABILITY_SERVICE_URL=http://localhost:4103` is wrong
  inside a container (the service listens on 8103 there). Change it to
  `http://localhost:8103`.
- **Every file:** if it has a `MIGRATION`/`*_MIGRATION` line, keep it as it is
  for now. The deploy step handles tables for gateway, dispatch and
  payment-wallet itself (see K3).

### K2. Adopt one service

```bash
bin/adopt.sh notification-service $R/notification-service:$C
```

**What you should see:**

```
stopping karrygo-notification-service and keeping it as karrygo-notification-service-pre-adoption-...
== deploying notification-service -> ghcr.io/.../notification-service:...
waiting for karrygo-notification-service to report healthy (up to 2 minutes)
== notification-service is healthy on ghcr.io/...
== notification-service is adopted and healthy.
The old containers are stopped but kept. Once you are happy, delete them with:
    docker rm karrygo-notification-service-pre-adoption-...
```

If it fails, it prints the new container's last log lines and **starts the old
one again**. Nothing is lost; read the log lines and fix the setting.

**Order:** notification → payment-wallet → media → verification → fleet →
dispatch → api-gateway (last, because everything goes through it).

### K3. Services whose tables the deploy prepares (gateway, dispatch, payment-wallet)

These three have no table-setup code of their own, so the deploy runs
`migrate.sh` first. Their databases already have tables (created by hand
before), so **before** adopting each one, tell the tracker which files are
already done:

```bash
# 1. See what the tracker thinks (nothing is changed):
docker run --rm --network karrygo-net --env-file /opt/karrygo/api-gateway/gateway.env \
  -e MIGRATE_DB_URL_VAR=API_GATEWAY_DATABASE_URL --entrypoint /srv/app/migrate.sh \
  $R/api-gateway:$C --status
```

Compare the PENDING list with the database. A quick look at a table:
`docker exec karrygo-api-gateway-db psql -U cosmicforge_logistics -d api_gateway -c '\dt'`
(adjust the user and database names to what's in the settings file).

- If **all** tables exist: add `--baseline` instead of `--status`.
- If the database is behind: add `--baseline --until <last file that is already applied>`.
  The deploy then applies only the remaining files.

Repeat for dispatch (`DISPATCH_DELIVERY_DATABASE_URL`, file `dispatch.env`) and
payment-wallet (`PAYMENT_WALLET_DATABASE_URL`, file `wallet.env`). Dispatch's
server folder has `migration-20261016-090932.log` and `pending-migrations.sql`
from the last manual migration: they show how far it got.

### K4. Dispatch's uploaded photos

`adopt.sh` does this automatically for dispatch. After stopping the old
container, it copies `/tmp/cosmicforge-avatar-uploads`, `-verification-uploads`
and `-trip-proof-uploads` onto the permanent volume `karrygo-dispatch-uploads`.
The links stored in the database are relative, so they keep working. You'll see
`copied /tmp/cosmicforge-avatar-uploads (N files)` lines.

### K5. Close out

```bash
bin/status.sh                     # every adopted service: running / healthy
docker rm <name>-pre-adoption-... # only after a day or two of everything working
```

Archive the old compose folders' files you no longer use (for example
`/opt/karrygo/api-gateway/docker-compose.yml` and `browser-origin.override.yml`):
rename them with `.retired` so nobody runs them by mistake. The new files are in
`/opt/karrygo/platform/compose/`.

---

## L. Add the new services

New: customer, support-dispute, referral, pricing, taxi, hauling. Each gets its
own database (and Redis where it needs one), created automatically on its first deploy.

For each one (example: customer):

```bash
sudo -iu deploy
mkdir -p /opt/karrygo/customer-service && cd /opt/karrygo/customer-service

# 1. database/Redis passwords
cp /opt/karrygo/platform/env/db.env.example db.env
nano db.env              # POSTGRES_DB=customer_service, passwords: openssl rand -hex 32
chmod 600 db.env

# 2. the service's settings
cp /opt/karrygo/platform/env/customer.env.example customer.env
nano customer.env        # fill every GENERATE / MUST MATCH / <db.env ...> (sections G and H)
chmod 600 customer.env

# 3. first deploy
/opt/karrygo/platform/bin/deploy-service.sh customer-service $R/customer-service:$C
```

| Service | Home folder | Settings file | `POSTGRES_DB` | Needs Redis |
|---|---|---|---|---|
| customer-service | `customer-service` | `customer.env` | `customer_service` | yes |
| support-dispute-service | `support-dispute-service` | `support.env` | `support_dispute_service` | no |
| referral-service | `referral-service` | `referral.env` | `referral_service` | yes |
| pricing-service | `pricing-service` | `pricing.env` | `pricing_service` | no |
| driver-taxi-service | `taxi-service` | `taxi.env` | `taxi_service` | yes |
| driver-hauling-service | `hauling-service` | `hauling.env` | `hauling_service` | yes |

(For services without Redis, delete the `REDIS_PASSWORD` line from `db.env`.)

**Then tell the gateway where they are.** Add to `/opt/karrygo/api-gateway/gateway.env`:

```
GATEWAY_UPSTREAM_CUSTOMER_URL=http://karrygo-customer-service:8101
GATEWAY_UPSTREAM_TAXI_URL=http://karrygo-taxi-service:8102
GATEWAY_UPSTREAM_HAULING_URL=http://karrygo-hauling-service:8104
GATEWAY_UPSTREAM_SUPPORT_URL=http://karrygo-support-dispute-service:8107
```

and apply it: `bin/apply-env.sh api-gateway`. Also add each new caller's secret
to notification, payment-wallet and media (section H), then apply those too.

Referral has no public route through the gateway yet (no app screen uses it).
Pricing is internal only. Neither needs a gateway line today.

---

## M. Tighten the front door (nginx)

The server's current site only blocks addresses that *start* with `/internal`.
The new file also blocks hidden `/internal/` paths, admin-only support pages,
internal statistics pages, and the non-public notification routes.

```bash
bash scripts/ssh-vps.sh
sudo cp /etc/nginx/sites-available/api-gateway.staging.karryam.com{,.bak-$(date +%F)}
sudo cp /opt/karrygo/platform/nginx/api-gateway.staging.karryam.com.conf \
        /etc/nginx/sites-available/api-gateway.staging.karryam.com
sudo nginx -t && sudo systemctl reload nginx
```

**What you should see:** `syntax is ok` and `test is successful`. If `nginx -t`
fails, nothing was reloaded; put the backup back with the reverse `cp`.

**Check from your Mac:**

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://api-gateway.staging.karryam.com/health                 # 200
curl -s -o /dev/null -w '%{http_code}\n' https://api-gateway.staging.karryam.com/api/v1/customer/internal/x  # 404
```

**HTTPS certificate renewal** is automatic (certbot). Test it with
`sudo certbot renew --dry-run`.

---

## N. Everyday: how a change reaches the server

1. Push your change to `staging` (or merge a pull request into it).
2. GitHub works out which services changed. A change in `shared/go` rebuilds
   all of them; a change only in `services/driver-hauling-service` rebuilds only hauling.
3. It tests, builds, and deploys them one by one: dependencies first, the gateway last.
4. Each deploy waits for "healthy"; a failure rolls back **that** service and
   marks the run red.
5. Finally it checks `https://api-gateway.staging.karryam.com/health` and that a
   private path returns 404.

**Deploy by hand from GitHub:** Actions → Deploy platform → **Run workflow** →
type `all` or, for example, `customer-service,driver-hauling-service`.
"Skip the Go tests" is for emergencies only.

vendor-service keeps its own workflow (`Deploy vendor-service`), which works the same way.

---

## O. Is it working? Health, logs, status

```bash
sudo -iu deploy
/opt/karrygo/platform/bin/status.sh          # one table: state, health, memory, version
docker logs --tail 100 -f karrygo-customer-service   # live log (Ctrl-C to stop)
curl -s localhost:8101/ready                 # new services answer on 127.0.0.1
docker stats --no-stream | grep karrygo      # memory/CPU per container
```

Every service has `/health` (the process is alive), `/ready` (its database and
other needs are reachable) and `/api/v1/<name>/meta` (version and commit).

---

## P. Rolling back

```bash
sudo -iu deploy
/opt/karrygo/platform/bin/rollback.sh payment-wallet-service            # to the previous version
/opt/karrygo/platform/bin/rollback.sh payment-wallet-service ghcr.io/cosmicforgehealthnet/payment-wallet-service:<commit>
```

It uses the same health check. Database table changes are not undone (they are
designed to only add things), so an older version keeps working on a newer schema.

---

## Q. Changing a setting

```bash
sudo -iu deploy
nano /opt/karrygo/customer-service/customer.env
/opt/karrygo/platform/bin/apply-env.sh customer-service
```

`docker restart` does **not** pick up a changed settings file: Docker copies the
values in when the container is created. `apply-env.sh` recreates the container
properly and waits for healthy.

---

## Push notifications (Firebase)

Push messages to phones go through Firebase Cloud Messaging, project
`logistics-93b9f`. notification-service needs two things, and only switches
push on when **both** are set:

1. `FIREBASE_PROJECT_ID=logistics-93b9f` in
   `/opt/karrygo/notification-service/notification.service.env`.
2. The Firebase Admin service-account file at
   `/opt/karrygo/notification-service/firebase-service-account.json`
   (mode 644; `/opt/karrygo` itself is deploy-only). The compose file mounts it
   read-only into the container and points `GOOGLE_APPLICATION_CREDENTIALS` at it.

The file is never committed (`credentials/` is git-ignored). To replace it,
for example after rotating the key in the Firebase console:

```bash
scp credentials/firebase.json deploy@173.249.30.55:/opt/karrygo/notification-service/firebase-service-account.json
ssh deploy@173.249.30.55 'chmod 644 /opt/karrygo/notification-service/firebase-service-account.json && /opt/karrygo/platform/bin/apply-env.sh notification-service'
```

Phones register their push token when someone logs in to an app, so a phone
only receives pushes after logging in on this server.

## R. Backups and restoring

**One-time setup** (as root):

```bash
sudo sh -c 'umask 077; cat > /root/.karrygo-backup-passphrase'   # type your passphrase, Enter, Ctrl-D
sudo bash /opt/karrygo/platform/bin/backup.sh                     # a first backup now
sudo bash /opt/karrygo/platform/bin/backup.sh --install-cron      # every night at 02:30
```

Each backup (in `/opt/karrygo/backups/<date-time>/`) holds:

- every KarryGo database (`karrygo-*-db.sql.gz`);
- all settings files, **encrypted** with your passphrase (`settings.tar.gz.enc`);
- dispatch's uploaded photos.

Seven days are kept.

**Prove a backup works** (do this monthly):

```bash
sudo bash /opt/karrygo/platform/bin/backup.sh --test-restore karrygo-payment-wallet-db
```

It restores the newest copy into a throwaway container, lists the databases with
their table counts, then deletes the container.

**Open the encrypted settings** (in a disaster):

```bash
openssl enc -d -aes-256-cbc -pbkdf2 -in settings.tar.gz.enc -pass file:/root/.karrygo-backup-passphrase | tar -xz
```

**Off-server copies:** install and configure `rclone`, then put
`RCLONE_REMOTE=<remote>:karrygo-backups` in `/etc/karrygo-backup.conf`. Backups
kept only on this server would be lost along with it.

---

## S. Monitoring

- **Uptime:** in UptimeRobot (free), add an HTTP monitor for
  `https://api-gateway.staging.karryam.com/health`, every 5 minutes, alerting your email/phone.
- **Disk:** `df -h /`. Keep it under 80%. Old images: `docker image prune -a --filter "until=336h"`
  (removes unused images older than 14 days; never touches running ones).
- **Memory:** `free -h` and `status.sh`. Each container has a memory limit, so one
  misbehaving service can't take the whole server down.
- **Logs** are capped at 5 × 20 MB per container, so they can't fill the disk.

---

## T. Security checklist

- [ ] Only ports 22, 80 and 443 answer from the internet (`nc -z` test from your Mac in step E).
- [ ] `close-public-ports.sh --check` shows the block and the boot unit.
- [ ] Every settings file is mode 600: `find /opt/karrygo -name '*.env' ! -perm 600`
      prints nothing.
- [ ] No debug login switches are on: `grep -l 'DEBUG_OTP=true\|OTP_DEBUG=true' /opt/karrygo/*/*.env`
      prints nothing.
- [ ] Password SSH login is off (F3); GitHub uses the `deploy` key only.
- [ ] nginx blocks `/internal/` anywhere in the address (M).
- [ ] Backups run nightly, the passphrase is in a password manager, and a restore was tested.
- [ ] fail2ban is active: `sudo fail2ban-client status sshd`.

---

## U. Troubleshooting

| What you see | Likely cause | What to do |
|---|---|---|
| Deploy says `<file> is missing` | The settings file isn't on the server | Create it (section G). |
| Deploy says `must have mode 600` | File readable by others | `chmod 600 <file>` |
| `refusing a reusable tag` | Tried to deploy `:latest`/`:staging` | Use the `:<commit>` image name. |
| `did not become healthy` + log lines | Wrong setting, database unreachable, or a code error | Read the log lines it printed; fix the setting; `apply-env.sh` or deploy again. It already rolled back. |
| `unauthorized` / `invalid signature` in logs | Two secrets that must match don't (section H) | Compare both values; apply both services. |
| `pull access denied` / `denied` | Server not logged in to GHCR | CI logs in itself; for manual deploys do the `docker login` in step J. |
| `Conflict. The container name ... is already in use` | A hand-started container still has that name | Adopt it with `adopt.sh` (section K), don't use `deploy-service.sh` first. |
| A migration fails during deploy | A table already exists (database was set up by hand) | Run `migrate.sh --status` and record the done files with `--baseline` (K3). |
| 502 Bad Gateway on the domain | The gateway container is down or unhealthy | `status.sh`, `docker logs karrygo-api-gateway` |
| 404 for an app route that should work | Gateway doesn't know the service's address | Add the `GATEWAY_UPSTREAM_*_URL` line (section L) and `apply-env.sh api-gateway`. |
| fleet (or another service) never gets "ready" after a deploy, logs mention migrations | The new version has table changes and that service's migration switch is off (`MIGRATION=false`) | Set its migration switch to `true` in its settings file and deploy again, or apply the migrations deliberately first. |
| Server slow / out of memory | Swap missing, or a container leaking | `free -h`, `docker stats`; run `server-setup.sh` (adds swap). |
| Certificate expired | certbot renewal failed | `sudo certbot renew` and read its message. |
| GitHub deploy job can't log in | Wrong `VPS_*` secrets or key | Re-set them (section I); test the key from your Mac (F2). |
| Dispatch photos missing after adoption | Copy step skipped | They are still in the stopped `...-pre-adoption-...` container: `docker cp <old>:/tmp/cosmicforge-avatar-uploads/. <dir>` then copy into the volume. |

---

## V. Pointing the mobile apps at the server

The apps default to a developer laptop's Wi-Fi address. For anyone else, build
them against the domain:

```bash
cd apps/customer
flutter build apk --release \
  --dart-define=CUSTOMER_API_BASE_URL=https://api-gateway.staging.karryam.com/api/v1/customer \
  --dart-define=HAULING_API_BASE_URL=https://api-gateway.staging.karryam.com/api/v1/hauling \
  --dart-define=DISPATCH_API_BASE_URL=https://api-gateway.staging.karryam.com/api/v1/customer/dispatch \
  --dart-define=WALLET_API_BASE_URL=https://api-gateway.staging.karryam.com/api/v1/payment-wallet \
  --dart-define=SUPPORT_API_BASE_URL=https://api-gateway.staging.karryam.com/api/v1/support-disputes \
  --dart-define=GOOGLE_MAPS_API_KEY=<your Maps key>
```

The exact setting names per app are in each app's config file
(`apps/customer/lib/core/config/customer_app_config.dart`,
`apps/truck_provider/lib/core/config/provider_app_config.dart`,
`apps/dispatch_provider/lib/core/config.dart`). The websocket address is
`wss://api-gateway.staging.karryam.com/api/v1/notifications/ws`.

---

## W. Reference: every service at a glance

| Service | Container | Inside port | Reachable on the server at | Tables prepared by |
|---|---|---|---|---|
| api-gateway | `karrygo-api-gateway` | 8080 | `127.0.0.1:4100` (nginx) | deploy (`migrate.sh`) |
| media-file-service | `karrygo-media-file-service` | 8109 | `127.0.0.1:4101` | itself (`MIGRATION`) |
| notification-service | `karrygo-notification-service` | 8106 | private network only | itself (`MIGRATION`) |
| driver-dispatch-delivery-service | `karrygo-dispatch-delivery-service` | 8103 | `127.0.0.1:4103` | deploy (`migrate.sh`) |
| payment-wallet-service | `karrygo-payment-wallet-service` | 8105 | private network only | deploy (`migrate.sh`) |
| verification-compliance-service | `karrygo-verification-staging` | 8111 | private network only | itself |
| fleet-partner-service (+ worker) | `karrygo-fleet-partner-api` | 8113 | `127.0.0.1:8113` (nginx) | itself (`MIGRATION`) |
| vendor-service (+ worker) | `karrygo-vendor-api-staging` | 8112 | `127.0.0.1:8112` (nginx) | its own workflow |
| customer-service | `karrygo-customer-service` | 8101 | `127.0.0.1:8101` | itself (`MIGRATION`) |
| driver-taxi-service | `karrygo-taxi-service` | 8102 | `127.0.0.1:8102` | itself (`TAXI_MIGRATION`) |
| driver-hauling-service | `karrygo-hauling-service` | 8104 | `127.0.0.1:8104` | itself (`HAULING_MIGRATION`) |
| support-dispute-service | `karrygo-support-dispute-service` | 8107 | `127.0.0.1:8107` | itself (`MIGRATION`) |
| referral-service | `karrygo-referral-service` | 8114 | `127.0.0.1:8114` | itself (`REFERRAL_MIGRATION`) |
| pricing-service | `karrygo-pricing-service` | 8115 | `127.0.0.1:8115` | itself (`PRICING_MIGRATION`) |

Not deployed (unfinished scaffolds): admin-backoffice-service, analytics-service.

**The files behind all of this:**

| File | Purpose |
|---|---|
| `deploy/platform/services.tsv` | The list of services: one row each, read by every script and the workflow |
| `deploy/platform/compose/*.yml` | One compose file per service; `common.yml` holds the shared safety settings |
| `deploy/platform/bin/deploy-service.sh` | Deploy one service safely (used by CI) |
| `deploy/platform/bin/adopt.sh` | Take over a hand-started service |
| `deploy/platform/bin/rollback.sh`, `apply-env.sh`, `status.sh` | Everyday commands |
| `deploy/platform/bin/migrate.sh` | Applies migrations once each (runs inside the image) |
| `deploy/platform/bin/export-container-env.sh` | Saves a running container's settings to a private file |
| `deploy/platform/bin/close-public-ports.sh`, `server-setup.sh`, `inventory.sh`, `backup.sh` | Server care |
| `deploy/platform/env/*.env.example` | Settings templates (no values) |
| `deploy/platform/nginx/` | The front-door site file |
| `.github/workflows/deploy-platform.yml` | The automatic pipeline |
