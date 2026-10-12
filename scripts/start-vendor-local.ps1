param([switch]$SkipBuild, [switch]$Stop)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$taskRuntime = Join-Path $taskRoot 'tmp/vendor-local'
$taskPidFile = Join-Path $taskRuntime 'pids.json'
$taskExisting = @()
if (Test-Path -LiteralPath $taskPidFile) {
  $taskRecords = Get-Content -LiteralPath $taskPidFile -Raw | ConvertFrom-Json
  if ($taskRecords.PSObject.Properties.Name -contains 'value') { $taskRecords = $taskRecords.value }
  foreach ($taskEntry in $taskRecords) {
    $taskProcess = Get-Process -Id $taskEntry.Pid -ErrorAction SilentlyContinue
    if ($taskProcess -and $taskProcess.Path -eq $taskEntry.Path) {
      if ($Stop) { Stop-Process -Id $taskProcess.Id }
      else { $taskExisting += $taskEntry }
    }
  }
}
if ($Stop) { Write-Output 'Local Wallet/Vendor/Media processes stopped.'; return }
$taskProcesses = @()
$taskMediaEnv = Join-Path $taskRuntime 'media.env'
if ((Test-Path -LiteralPath $taskMediaEnv) -and
    (Select-String -LiteralPath $taskMediaEnv -Pattern '^MEDIA_FILE_SCANNER_ADDRESS=127.0.0.1:3310$' -Quiet)) {
  & (Join-Path $PSScriptRoot 'start-vendor-scanner.ps1')
}

foreach ($taskService in @(@{Name='wallet';Directory='payment-wallet-service'}, @{Name='media';Directory='media-file-service'}, @{Name='verification';Directory='verification-compliance-service'}, @{Name='vendor';Directory='vendor-service'})) {
  $taskRunning = $taskExisting | Where-Object { $_.Name -eq $taskService.Name } | Select-Object -First 1
  if ($taskRunning) {
    $taskProcesses += $taskRunning
    Write-Output ($taskService.Name + ' already running, PID ' + $taskRunning.Pid)
    continue
  }
  $taskEnvFile = Join-Path $taskRuntime ($taskService.Name + '.env')
  if (!(Test-Path -LiteralPath $taskEnvFile)) { throw "Missing local configuration: $taskEnvFile" }
  $taskBinary = Join-Path $taskRuntime ($taskService.Name + '.exe')
  $taskDirectory = Join-Path $taskRoot ('services/' + $taskService.Directory)
  if (!$SkipBuild) {
    Push-Location $taskDirectory
    try { & go build -o $taskBinary ./cmd; if ($LASTEXITCODE -ne 0) { throw 'Build failed' } }
    finally { Pop-Location }
  }
  $taskPrevious = @{}
  try {
    foreach ($taskLine in Get-Content -LiteralPath $taskEnvFile) {
      if ($taskLine -match '^([^#=]+)=(.*)$') {
        $taskKey = $Matches[1]; $taskValue = $Matches[2]
        $taskPrevious[$taskKey] = [Environment]::GetEnvironmentVariable($taskKey, 'Process')
        [Environment]::SetEnvironmentVariable($taskKey, $taskValue, 'Process')
      }
    }
    if ($taskService.Name -eq 'wallet') {
      Push-Location $taskDirectory
      try {
        & go run ./tools/migrate
        if ($LASTEXITCODE -ne 0) { throw 'Wallet migration failed' }
      } finally { Pop-Location }
    }
    $taskProcess = Start-Process -FilePath $taskBinary -WorkingDirectory $taskDirectory -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskRuntime ($taskService.Name + '.out.log')) -RedirectStandardError (Join-Path $taskRuntime ($taskService.Name + '.err.log'))
    $taskProcesses += @{Name=$taskService.Name;Pid=$taskProcess.Id;Path=$taskBinary}
    Write-Output ($taskService.Name + ' started, PID ' + $taskProcess.Id)
  } finally {
    foreach ($taskKey in $taskPrevious.Keys) { [Environment]::SetEnvironmentVariable($taskKey, $taskPrevious[$taskKey], 'Process') }
  }
}
$taskProcesses | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $taskRuntime 'pids.json')
