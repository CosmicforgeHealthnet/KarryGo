#Requires -Version 5.1
<#
.SYNOPSIS
    Run the Dispatch Provider Flutter app on a physical Android device over USB,
    tunnelling both the dispatch backend (8103) and wallet backend (8105) through
    adb reverse so the phone can reach both services.

.DESCRIPTION
    1. Verifies adb is available.
    2. Lists connected devices so you can pick the right one.
    3. Sets up adb reverse tcp:8103 tcp:8103 (dispatch service).
    4. Sets up adb reverse tcp:8105 tcp:8105 (payment-wallet service).
    5. Runs `flutter run` with dart-defines for both services pointing to 127.0.0.1.

    Must be run from the apps/dispatch_provider directory (or pass -ProjectRoot).

.PARAMETER DeviceId
    ADB device serial number.  Defaults to RFCY51N8EJB (Samsung test device).
    Run `adb devices` to list all attached devices.

.PARAMETER Port
    Dispatch backend port.  Defaults to 8103.

.PARAMETER WalletPort
    Payment-wallet service port.  Defaults to 8105.

.PARAMETER DisableDds
    Add --disable-dds to the flutter run command.  Use when you hit a
    "service protocol" or DDS-conflict error.

.PARAMETER ProjectRoot
    Path to apps/dispatch_provider.  Defaults to the current directory.

.EXAMPLE
    # Run with the default device
    .\scripts\run_android_usb_local.ps1

.EXAMPLE
    # Run with a different device
    .\scripts\run_android_usb_local.ps1 -DeviceId ABC123DEF456

.EXAMPLE
    # If flutter run hangs with a DDS error
    .\scripts\run_android_usb_local.ps1 -DisableDds
#>
param(
    [string]$DeviceId    = 'RFCY51N8EJB',
    [int]   $Port        = 8103,
    [int]   $WalletPort  = 8105,
    [switch]$DisableDds,
    [string]$ProjectRoot = $PSScriptRoot + '\..'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Helpers ─────────────────────────────────────────────────────────────────

function Write-Step ([string]$msg) { Write-Host "`n▶  $msg" -ForegroundColor Cyan }
function Write-Ok   ([string]$msg) { Write-Host "   ✓ $msg"  -ForegroundColor Green }
function Write-Warn ([string]$msg) { Write-Host "   ⚠  $msg"  -ForegroundColor Yellow }
function Write-Err  ([string]$msg) { Write-Host "   ✗ $msg"  -ForegroundColor Red }

# ── 1. Check adb ─────────────────────────────────────────────────────────────

Write-Step "Checking adb..."
try {
    $null = Get-Command adb -ErrorAction Stop
    Write-Ok "adb found: $((Get-Command adb).Source)"
} catch {
    Write-Err "adb not found.  Install Android Platform Tools and add to PATH."
    Write-Host "    https://developer.android.com/tools/releases/platform-tools" -ForegroundColor DarkGray
    exit 1
}

# ── 2. List devices ───────────────────────────────────────────────────────────

Write-Step "Connected ADB devices:"
adb devices

# ── 3. Verify the target device is listed ─────────────────────────────────────

Write-Step "Checking device '$DeviceId'..."
$deviceList = adb devices
if ($deviceList -notmatch $DeviceId) {
    Write-Warn "Device '$DeviceId' not found in `adb devices` output."
    Write-Host "   Make sure USB Debugging is enabled on the phone and the USB" -ForegroundColor DarkGray
    Write-Host "   cable is connected.  Authorize the laptop on the phone if prompted." -ForegroundColor DarkGray
    Write-Host ""
    Write-Host "   To use a different device, pass -DeviceId <serial>:" -ForegroundColor DarkGray
    Write-Host "   .\scripts\run_android_usb_local.ps1 -DeviceId <YOUR_DEVICE_SERIAL>" -ForegroundColor DarkGray
    exit 1
}
Write-Ok "Device found."

# ── 4. Set up adb reverse (dispatch service) ─────────────────────────────────

Write-Step "Setting up adb reverse tcp:$Port tcp:$Port (dispatch)..."
try {
    adb -s $DeviceId reverse tcp:$Port tcp:$Port
    Write-Ok "adb reverse active — phone's 127.0.0.1:$Port → laptop's localhost:$Port"
} catch {
    Write-Err "adb reverse failed: $_"
    exit 1
}

# ── 4b. Set up adb reverse (payment-wallet service) ──────────────────────────

Write-Step "Setting up adb reverse tcp:$WalletPort tcp:$WalletPort (payment-wallet)..."
try {
    adb -s $DeviceId reverse tcp:$WalletPort tcp:$WalletPort
    Write-Ok "adb reverse active — phone's 127.0.0.1:$WalletPort → laptop's localhost:$WalletPort"
} catch {
    Write-Err "adb reverse failed for wallet port: $_"
    exit 1
}

Write-Step "Verifying adb reverse..."
$reverseList = adb -s $DeviceId reverse --list
Write-Host "   $reverseList" -ForegroundColor DarkGray

# ── 5. Build flutter run arguments ───────────────────────────────────────────

$apiUrl       = "http://127.0.0.1:$Port"
$walletApiUrl = "http://127.0.0.1:$WalletPort/api/v1/payment-wallet"

$flutterArgs = @(
    'run',
    '-d', $DeviceId,
    "--dart-define=DISPATCH_PROVIDER_API_BASE_URL=$apiUrl",
    "--dart-define=DISPATCH_PROVIDER_WALLET_BASE_URL=$walletApiUrl"
)
if ($DisableDds) {
    $flutterArgs += '--disable-dds'
    Write-Warn "--disable-dds enabled (DDS conflict workaround)"
}

# ── 6. Navigate to project root and run ──────────────────────────────────────

$resolvedRoot = Resolve-Path $ProjectRoot -ErrorAction Stop

Write-Step "Running flutter from: $resolvedRoot"
Write-Host ""
Write-Host "   Command:" -ForegroundColor DarkGray
Write-Host "   flutter $($flutterArgs -join ' ')" -ForegroundColor DarkGray
Write-Host ""
Write-Host "   Expected debug log:" -ForegroundColor DarkGray
Write-Host "   [CONFIG] Dispatch Provider API base URL: $apiUrl" -ForegroundColor DarkGray
Write-Host "   [CONFIG] Backend mode hint: physical-phone-usb (...)" -ForegroundColor DarkGray
Write-Host "   [CONFIG] Backend health ... reachable=true ..." -ForegroundColor DarkGray
Write-Host ""

Push-Location $resolvedRoot
try {
    & flutter @flutterArgs
} finally {
    Pop-Location
}
