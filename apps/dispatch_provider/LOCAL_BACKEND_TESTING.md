# Dispatch Provider — Local Backend Testing

## Quick Reference

| Scenario | Base URL to use | Extra step |
|---|---|---|
| Chrome / Windows desktop | `http://localhost:8103` | none |
| Android emulator | `http://10.0.2.2:8103` | none |
| Physical Android — USB (recommended) | `http://127.0.0.1:8103` | `adb reverse tcp:8103 tcp:8103` |
| Physical Android — Wi-Fi | `http://192.168.x.x:8103` (your laptop IP) | backend firewall open on 8103 |
| Production / VPS | `https://api.yourdomain.com` | deploy backend |

> **⚠️ Common mistake:** Running the app on a physical phone *without* `--dart-define`.
> The app then defaults to `http://localhost:8103`.  On the phone, `localhost` is the
> phone itself — not the laptop — so every API call fails with "Cannot connect".

---

## 1 — Start the Backend

From the repo root (`C:\KarryGoPush`):

```powershell
docker compose -f infra/docker-compose.yml up --build -d driver-dispatch-delivery-service
```

Verify it's healthy:

```powershell
curl.exe http://localhost:8103/health
curl.exe http://localhost:8103/ready
```

Watch backend logs live:

```powershell
docker compose -f infra/docker-compose.yml logs -f driver-dispatch-delivery-service
```

---

## 2 — Physical Android over USB (Recommended)

**Device ID used for testing: `RFCY51N8EJB`** (Samsung Galaxy S series)

### Step 1 — Connect USB and enable adb

```powershell
adb devices
# Expected: RFCY51N8EJB   device
```

### Step 2 — Tunnel backend port to phone

```powershell
adb reverse tcp:8103 tcp:8103
adb reverse --list        # verify: 8103 -> 8103 appears
```

This makes `127.0.0.1:8103` on the *phone* resolve to port 8103 on the *laptop*.

### Step 3 — Run the app

```powershell
flutter run -d RFCY51N8EJB `
  --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
```

If you get a "service protocol" or "DDS" error:

```powershell
flutter run -d RFCY51N8EJB --disable-dds `
  --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://127.0.0.1:8103
```

### What to look for in the debug log

```
[CONFIG] Dispatch Provider API base URL: http://127.0.0.1:8103
[CONFIG] Backend mode hint: physical-phone-usb (adb reverse tcp:8103 tcp:8103 required)
[CONFIG] Backend health base=http://127.0.0.1:8103 reachable=true status=200 error=none
```

If you see `reachable=false` — the tunnel is not active.  Re-run `adb reverse tcp:8103 tcp:8103`.

---

## 3 — Physical Android over Wi-Fi

Use this only when USB is unavailable.  The laptop IP may change if DHCP reassigns.

```powershell
# Find your laptop Wi-Fi IP
ipconfig | Select-String "IPv4"

# Run with that IP
flutter run -d RFCY51N8EJB `
  --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://192.168.x.x:8103
```

The phone and laptop must be on the same Wi-Fi network.  The backend must accept
connections on port 8103 from LAN addresses (no firewall block).

---

## 4 — Android Emulator

`10.0.2.2` is the emulator's alias for the host machine's localhost:

```powershell
flutter run `
  --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://10.0.2.2:8103
```

---

## 5 — Chrome / Windows Desktop

No special setup.  `localhost` in Chrome means the same machine:

```powershell
flutter run -d chrome `
  --dart-define=DISPATCH_PROVIDER_API_BASE_URL=http://localhost:8103
```

If you omit `--dart-define` entirely the app also uses `http://localhost:8103`,
which works fine on desktop only.

---

## 6 — URL Quick Notes

- `localhost` / `127.0.0.1` on a **physical phone** → the phone's own loopback.
  Backend NOT reachable without `adb reverse`.
- `10.0.2.2` → only valid inside the Android emulator.
- `192.168.x.x` → LAN IP.  Changes if DHCP reassigns; use a reservation or static IP.
- `https://…` → production/VPS; no local tunnel needed.

---

## 7 — Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| "Cannot connect to KarryGo server" | localhost used on physical phone | `adb reverse tcp:8103 tcp:8103` and re-run with `--dart-define` |
| `reachable=false` in log despite USB | adb reverse not applied to this session | Unplug and replug USB, re-run `adb reverse`, check `adb devices` |
| `adb: error: no devices/emulators found` | USB debug not enabled or not authorized | On phone: Settings > Developer Options > USB Debugging |
| `flutter run` hangs / DDS error | DDS conflict with existing Flutter tooling | Add `--disable-dds` |
| Backend health 503 | Backend starting up | Wait 5–10 seconds and retry `curl.exe http://localhost:8103/health` |
