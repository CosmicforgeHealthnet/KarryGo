param([int]$FromIndex=1)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskEvidence=Join-Path $taskRoot 'docs/validation/sections911-race-results'
New-Item -ItemType Directory -Path $taskEvidence -Force|Out-Null
$taskGroups=@(
 @{Template='closure_tpl_customer';Env='CUSTOMER_TEST_DATABASE_URL';Paths=@('./services/customer-service/internal/features/journey_sharing/...','./services/customer-service/internal/features/movement_history/...')},
 @{Template='closure2_tpl_taxi';Env='TAXI_TEST_DATABASE_URL';Paths=@('./services/driver-taxi-service/internal/features/customertracking','./services/driver-taxi-service/internal/features/goods')},
 @{Template='closure_tpl_dispatch';Env='DISPATCH_TEST_DATABASE_URL';Paths=@('./services/driver-dispatch-delivery-service/internal/features/customershare','./services/driver-dispatch-delivery-service/internal/features/availability','./services/driver-dispatch-delivery-service/internal/features/movement')},
 @{Template='vendor_closure_b5';Env='VENDOR_SERVICE_TEST_DATABASE_URL';Paths=@('./services/vendor-service/internal/features/customershare','./services/vendor-service/internal/features/fulfilmentgroup','./services/vendor-service/cmd')},
 @{Template='closure_stack_tpl_hauling';Env='HAULING_TEST_DATABASE_URL';Paths=@('./services/driver-hauling-service/internal/features/customershare','./services/driver-hauling-service/internal/features/provider_availability/usecases','./services/driver-hauling-service/internal/features/movement')},
 @{Template='sections911';Env='MOVEMENT_TEST_DATABASE_URL';Paths=@('./shared/go/journeyshare/...','./shared/go/events')}
)
$taskIndex=0;$taskResults=@()
if($FromIndex -gt 1){$taskResults=@(Get-Content (Join-Path $taskEvidence 'race-summary.json') -Raw|ConvertFrom-Json)}
foreach($taskGroup in $taskGroups){foreach($taskPackage in $taskGroup.Paths){
 $taskIndex++;if($taskIndex -lt $FromIndex){continue};$taskDb='closure_race_'+$taskIndex
 & docker exec karry-customer-sections911-postgres createdb -U karry_test -T $taskGroup.Template $taskDb 2>&1|Out-File (Join-Path $taskEvidence ('setup-'+$taskIndex+'.log'))
 if($LASTEXITCODE -ne 0){throw ('Race database setup failed: '+$taskGroup.Template)}
 $taskDsn='postgres://karry_test:karry_test@host.docker.internal:55432/'+$taskDb+'?sslmode=disable'
 $taskLog=Join-Path $taskEvidence ($taskIndex.ToString()+'.log')
 & docker exec -e CGO_ENABLED=1 -e ($taskGroup.Env+'='+$taskDsn) -e JOURNEY_TEST_REDIS_ADDR=host.docker.internal:56379 -e TAXI_TEST_REDIS_ADDR=host.docker.internal:56379 -e VENDOR_SERVICE_TEST_REDIS_ADDR=host.docker.internal:56379 karry-sections911-cgo go test -race $taskPackage -json -count=1 -p 1 -timeout 12m 2>&1|Out-File $taskLog -Encoding utf8
 $taskExit=$LASTEXITCODE;$taskEvents=@(Get-Content $taskLog|ForEach-Object{try{$_|ConvertFrom-Json}catch{}})
 $taskResults += [pscustomobject]@{Package=$taskPackage;Exit=$taskExit;Status=$(if($taskExit -eq 0){'PASS'}else{'FAIL'});Pass=@($taskEvents|Where-Object{$_.Action -eq 'pass' -and $_.Test}).Count;Fail=@($taskEvents|Where-Object{$_.Action -eq 'fail' -and $_.Test}).Count;Skipped=@($taskEvents|Where-Object{$_.Action -eq 'skip' -and $_.Test}).Count;Evidence=[IO.Path]::GetFileName($taskLog)}
 $taskResults|ConvertTo-Json -Depth 4|Set-Content (Join-Path $taskEvidence 'race-summary.json')
 Write-Output ($taskResults[-1]|ConvertTo-Json -Compress)
}}
