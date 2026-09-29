# Projection outage/recovery on the running local stack: stop Taxi while a
# Ride share is live, observe the typed public failure, restart, observe recovery.
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskRuntime=Join-Path $taskRoot 'tmp/sections911'
$taskConfig=Get-Content (Join-Path $taskRuntime 'local-config.json') -Raw|ConvertFrom-Json
$taskReady=Join-Path $taskRuntime 'outage-ready'
Remove-Item $taskReady -ErrorAction SilentlyContinue
$taskEvidence=Join-Path $PSScriptRoot 'sections911-stack-results'
New-Item -ItemType Directory -Path $taskEvidence -Force|Out-Null
$taskJob=Start-Job -ScriptBlock {
 param($root,$secret,$ready,$log)
 $env:SECTIONS911_STACK_E2E='1';$env:SECTIONS911_STACK_OUTAGE='1';$env:SECTIONS911_STACK_SECRET=$secret;$env:SECTIONS911_STACK_OUTAGE_READY=$ready
 Set-Location (Join-Path $root 'shared/go')
 & go test ./journeyshare/journeysharetest/stacke2e/ -run '^TestStackProjectionOutage$' -count=1 -v 2>&1 | Tee-Object -FilePath $log
} -ArgumentList $taskRoot,$taskConfig.Secret,$taskReady,(Join-Path $taskEvidence 'TestStackProjectionOutage.log')
$taskDeadline=(Get-Date).AddMinutes(3)
while(-not (Test-Path $taskReady)){ if((Get-Date) -gt $taskDeadline){throw 'share never became ready'}; Start-Sleep -Milliseconds 300 }
$taskTaxi=(Get-Content (Join-Path $taskRuntime 'pids.json') -Raw|ConvertFrom-Json)|Where-Object {$_.Name -eq 'driver-taxi-service'}
Stop-Process -Id $taskTaxi.Pid
Start-Sleep -Seconds 8
& (Join-Path $PSScriptRoot 'sections911-local-stack.ps1') -EnableTestFeatures
Receive-Job -Job $taskJob -Wait -AutoRemoveJob | Select-Object -Last 12
