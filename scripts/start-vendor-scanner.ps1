param([switch]$UpdateSignatures)
$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$taskRuntime = Join-Path $taskRoot 'tmp/vendor-local/clamav-native'
$taskBinary = Join-Path $taskRuntime 'clamav-1.4.6.win.x64/clamd.exe'
$taskPidFile = Join-Path $taskRuntime 'pid.json'
if (!(Test-Path -LiteralPath $taskBinary)) { throw 'Native ClamAV package is missing' }
if ($UpdateSignatures) {
  & (Join-Path $taskRuntime 'clamav-1.4.6.win.x64/freshclam.exe') ('--config-file=' + (Join-Path $taskRuntime 'freshclam.conf')) --quiet
  if ($LASTEXITCODE -ne 0) { throw 'Signature update failed' }
}
if (Test-Path -LiteralPath $taskPidFile) {
  $taskRecord = Get-Content -LiteralPath $taskPidFile -Raw | ConvertFrom-Json
  $taskExisting = Get-Process -Id $taskRecord.Pid -ErrorAction SilentlyContinue
  if ($taskExisting -and $taskExisting.Path -eq $taskBinary) { Write-Output 'Native scanner already running'; return }
}
$taskProcess = Start-Process -FilePath $taskBinary -ArgumentList ('--config-file="' + (Join-Path $taskRuntime 'clamd.conf') + '"') -WorkingDirectory $taskRuntime -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskRuntime 'scanner.out.log') -RedirectStandardError (Join-Path $taskRuntime 'scanner.err.log')
@{Pid=$taskProcess.Id;Path=$taskBinary} | ConvertTo-Json | Set-Content -LiteralPath $taskPidFile
Write-Output ('Native scanner started, PID ' + $taskProcess.Id)
