param([switch]$EnableTestFeatures,[switch]$Stop)
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskRuntime=Join-Path $taskRoot 'tmp/sections911'
New-Item -ItemType Directory -Path $taskRuntime -Force|Out-Null
$taskPidFile=Join-Path $taskRuntime 'pids.json'
if(Test-Path $taskPidFile){foreach($taskEntry in (Get-Content $taskPidFile -Raw|ConvertFrom-Json)){ $taskProcess=Get-Process -Id $taskEntry.Pid -ErrorAction SilentlyContinue;if($taskProcess -and $taskProcess.Path -eq $taskEntry.Path){Stop-Process -Id $taskEntry.Pid} }}
if($Stop){exit}
# All credentials/origins below are disposable local validation fixtures.
# This script does not discover, modify or certify any deployed staging config.
$taskConfigFile=Join-Path $taskRuntime 'local-config.json'
if(Test-Path $taskConfigFile){$taskConfig=Get-Content $taskConfigFile -Raw|ConvertFrom-Json}else{
 $taskBytes=New-Object byte[] 32;[Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($taskBytes);$taskSecret=($taskBytes|ForEach-Object {$_.ToString("x2")}) -join ""
 $taskConfig=@{Secret=$taskSecret};$taskConfig|ConvertTo-Json|Set-Content $taskConfigFile
}
$taskSecret=$taskConfig.Secret
$taskEnabled=if($EnableTestFeatures){'true'}else{'false'}
$taskDefaults=@{WALLET_SERVICE_SECRET=$taskSecret;VENDOR_SERVICE_ENABLED='true';APP_ENV='test';GIN_MODE='release';CUSTOMER_ACCESS_TOKEN_SECRET=$taskSecret;CUSTOMER_REFRESH_TOKEN_SECRET=$taskSecret;CUSTOMER_OTP_SECRET=$taskSecret;CUSTOMER_SERVICE_SECRETS=('driver-taxi-service='+$taskSecret+',driver-dispatch-delivery-service='+$taskSecret+',driver-hauling-service='+$taskSecret+',vendor-service='+$taskSecret+',admin-backoffice-service='+$taskSecret);TAXI_SERVICE_SECRETS=('customer-service='+$taskSecret+',vendor-service='+$taskSecret);HAULING_SERVICE_SECRETS=('customer-service='+$taskSecret+',vendor-service='+$taskSecret);DISPATCH_SERVICE_SECRETS=('customer-service='+$taskSecret);VENDOR_SERVICE_SECRETS=('customer-service='+$taskSecret);VENDOR_SERVICE_SECRET=$taskSecret;TAXI_PROVIDER_TOKEN_SECRET=$taskSecret;TAXI_PROVIDER_REFRESH_SECRET=$taskSecret;TAXI_PROVIDER_OTP_SECRET=$taskSecret;TAXI_CUSTOMER_TOKEN_SECRET=$taskSecret;HAULING_PROVIDER_TOKEN_SECRET=$taskSecret;HAULING_PROVIDER_REFRESH_SECRET=$taskSecret;HAULING_PROVIDER_OTP_SECRET=$taskSecret;HAULING_CUSTOMER_TOKEN_SECRET=$taskSecret;DISPATCH_RIDER_ACCESS_TOKEN_SECRET=$taskSecret;DISPATCH_RIDER_REFRESH_TOKEN_SECRET=$taskSecret;DISPATCH_CUSTOMER_TOKEN_SECRET=$taskSecret;DISPATCH_RIDER_OTP_SECRET=$taskSecret;VENDOR_AUTH_ACCESS_TOKEN_SECRET=$taskSecret;VENDOR_AUTH_REFRESH_TOKEN_SECRET=$taskSecret;VENDOR_AUTH_OTP_SECRET=$taskSecret;VENDOR_CUSTOMER_ACCESS_TOKEN_SECRET=$taskSecret;VENDOR_PROCESS_MODE='api';CUSTOMER_SHARE_JOURNEY_ENABLED=$taskEnabled;TAXI_CUSTOMER_LIVE_LOCATION_ENABLED=$taskEnabled;CUSTOMER_SHARE_VIEWER_ORIGIN='http://127.0.0.1:58101';CUSTOMER_SHARE_TRUSTED_GATEWAY_CIDRS='127.0.0.1/32';CUSTOMER_SHARE_JOURNEY_TTL_SECONDS='3600';CUSTOMER_SHARE_COMPLETED_RETENTION_SECONDS='60';CUSTOMER_SHARE_TAXI_URL='http://127.0.0.1:58102';CUSTOMER_SHARE_DISPATCH_URL='http://127.0.0.1:58103';CUSTOMER_SHARE_HAULING_URL='http://127.0.0.1:58104';CUSTOMER_SHARE_VENDOR_URL='http://127.0.0.1:58112';CUSTOMER_SHARE_TAXI_SECRET=$taskSecret;CUSTOMER_SHARE_DISPATCH_SECRET=$taskSecret;CUSTOMER_SHARE_HAULING_SECRET=$taskSecret;CUSTOMER_SHARE_VENDOR_SECRET=$taskSecret;VENDOR_DISPATCH_URL='http://127.0.0.1:58103';VENDOR_DISPATCH_SECRET=$taskSecret;VENDOR_HAULING_URL='http://127.0.0.1:58104';VENDOR_HAULING_SECRET=$taskSecret;DISPATCH_COMMERCE_INTEGRATION_ENABLED='true';HAULING_COMMERCE_INTEGRATION_ENABLED='true';HAULING_SECURE_HANDOVER_REQUIRED='false';VENDOR_OWN_DELIVERY_ENABLED='true';CUSTOMER_MOBILITY_PREFERENCES_ENABLED='true';CUSTOMER_GUEST_TRAVELLERS_ENABLED='true';CUSTOMER_TRUSTED_CONTACTS_ENABLED='true';CUSTOMER_UNIFIED_MOVEMENT_HISTORY_ENABLED='true';CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED='true';TAXI_GOODS_DELIVERY_ENABLED='true';TAXI_GOODS_PRICING='{"base_minor":1000,"per_kilometre_minor":200,"per_minute_minor":10,"handling_minor":50,"minimum_minor":1500,"currency":"NGN","version":"local-v1"}';TAXI_GOODS_HANDOVER_SECRET=$taskSecret;VENDOR_TAXI_URL='http://127.0.0.1:58102';VENDOR_TAXI_SECRET=$taskSecret;TAXI_MEDIA_URL='http://127.0.0.1:58199';TAXI_MEDIA_TOKEN=$taskSecret;TAXI_PAYMENT_URL='http://127.0.0.1:58199';TAXI_PAYMENT_SECRET=$taskSecret;TAXI_VENDOR_URL='http://127.0.0.1:58112';TAXI_VENDOR_SECRET=$taskSecret;DISPATCH_RIDER_DEBUG_OTP='true';TAXI_CUSTOMER_MOVEMENT_EVENTS_ENABLED=$taskEnabled;TAXI_CUSTOMER_SERVICE_URL='http://127.0.0.1:58101';TAXI_CUSTOMER_SERVICE_SECRET=$taskSecret;DISPATCH_CUSTOMER_MOVEMENT_EVENTS_ENABLED=$taskEnabled;DISPATCH_CUSTOMER_SERVICE_URL='http://127.0.0.1:58101';DISPATCH_CUSTOMER_SERVICE_SECRET=$taskSecret;HAULING_CUSTOMER_MOVEMENT_EVENTS_ENABLED=$taskEnabled;HAULING_CUSTOMER_SERVICE_URL='http://127.0.0.1:58101';HAULING_CUSTOMER_SERVICE_SECRET=$taskSecret;VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED=$taskEnabled;VENDOR_CUSTOMER_URL='http://127.0.0.1:58101';VENDOR_CUSTOMER_SECRET=$taskSecret}
foreach($taskKey in $taskDefaults.Keys){[Environment]::SetEnvironmentVariable($taskKey,$taskDefaults[$taskKey],'Process')}
$taskServices=@(
 @{Name='customer-service';Template='closure_tpl_customer';Db='closure_stack_customer';Port='58101';DbEnv='CUSTOMER_DATABASE_URL';RedisEnv='CUSTOMER_REDIS_ADDR'},
 @{Name='driver-taxi-service';Template='closure2_tpl_taxi';Db='closure_stack_taxi';Port='58102';DbEnv='TAXI_DATABASE_URL';RedisEnv='TAXI_REDIS_ADDR'},
 @{Name='driver-dispatch-delivery-service';Template='closure_tpl_dispatch';Db='closure_stack_dispatch';Port='58103';DbEnv='DISPATCH_DELIVERY_DATABASE_URL';RedisEnv='DISPATCH_DELIVERY_REDIS_ADDR'},
 @{Name='driver-hauling-service';Template='closure_stack_tpl_hauling';Db='closure_stack_hauling';Port='58104';DbEnv='HAULING_DATABASE_URL';RedisEnv='HAULING_REDIS_ADDR'},
 @{Name='vendor-service';Template='vendor_closure_b5';Db='closure_stack_vendor';Port='58112';DbEnv='VENDOR_DATABASE_URL';RedisEnv='VENDOR_REDIS_ADDR'}
)
$taskPids=@()
foreach($taskService in $taskServices){
 $taskExists=& docker exec karry-customer-sections911-postgres psql -U karry_test -d postgres -Atc ('SELECT count(*) FROM pg_database WHERE datname='''+$taskService.Db+'''')
 if($taskExists -ne '1'){& docker exec karry-customer-sections911-postgres createdb -U karry_test -T $taskService.Template $taskService.Db;if($LASTEXITCODE -ne 0){throw 'Local stack database clone failed'}}
 [Environment]::SetEnvironmentVariable($taskService.DbEnv,('postgres://karry_test:karry_test@127.0.0.1:55432/'+$taskService.Db+'?sslmode=disable'),'Process')
 [Environment]::SetEnvironmentVariable($taskService.RedisEnv,'127.0.0.1:56379','Process')
 # Dispatch only skips mandatory OTP delivery in development; the E2E driver
 # logs the seeded rider in with the local debug OTP.
 if($taskService.Name -eq 'driver-dispatch-delivery-service'){$env:APP_ENV='development'}else{$env:APP_ENV='test'}
 if($taskService.Name -eq 'driver-hauling-service'){$env:HAULING_COMMERCE_INTEGRATION_ENABLED='false'}else{$env:HAULING_COMMERCE_INTEGRATION_ENABLED='true'}
 $env:HTTP_ADDR='127.0.0.1:'+$taskService.Port
 $taskBinary=Join-Path $taskRuntime ($taskService.Name+'.exe')
 $taskProcess=Start-Process -FilePath $taskBinary -WorkingDirectory (Join-Path $taskRoot ('services/'+$taskService.Name)) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskRuntime ($taskService.Name+'.log')) -RedirectStandardError (Join-Path $taskRuntime ($taskService.Name+'.stderr.log'))
 $taskPids += @{Name=$taskService.Name;Pid=$taskProcess.Id;Path=$taskBinary}
 $taskPids|ConvertTo-Json|Set-Content $taskPidFile
}
Write-Output ('Started '+$taskPids.Count+' local service processes; test features enabled='+[bool]$EnableTestFeatures)
