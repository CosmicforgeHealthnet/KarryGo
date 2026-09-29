param([string]$Run='.',[string]$Load='')
# Runs the black-box journey-sharing E2E/load driver against the RUNNING local
# stack started by sections911-local-stack.ps1 -EnableTestFeatures.
$ErrorActionPreference='Stop'
$taskRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskConfig=Get-Content (Join-Path $taskRoot 'tmp/sections911/local-config.json') -Raw|ConvertFrom-Json
$env:SECTIONS911_STACK_E2E='1'
$env:SECTIONS911_STACK_SECRET=$taskConfig.Secret
$env:SECTIONS911_STACK_LOAD=$Load
$taskEvidence=Join-Path $PSScriptRoot 'sections911-stack-results'
New-Item -ItemType Directory -Path $taskEvidence -Force|Out-Null
# Each driver process logs the seeded rider in once; reset only that local
# fixture's OTP request counter so repeated runs stay under Dispatch's limit.
& docker exec karry-customer-sections911-redis redis-cli DEL 'dispatch_rider_auth:otp_rate:+2348077770002' | Out-Null
Push-Location (Join-Path $taskRoot 'shared/go')
try{
 $taskLog=Join-Path $taskEvidence (($Run -replace '[^A-Za-z0-9]','_')+'.log')
 & go test ./journeyshare/journeysharetest/stacke2e/ -run $Run -count=1 -v -timeout 30m 2>&1 | Tee-Object -FilePath $taskLog | Select-Object -Last 40
 if($LASTEXITCODE -ne 0){ Write-Output 'STACK E2E FAILED' }
} finally { Pop-Location }
# Vendor Own Delivery needs Vendor's own runtime use cases for its transitions.
$env:VENDOR_SERVICE_TEST_DATABASE_URL='postgres://karry_test:karry_test@127.0.0.1:55432/closure_stack_vendor?sslmode=disable'
$env:SECTIONS911_STACK_CUSTOMER_DSN='postgres://karry_test:karry_test@127.0.0.1:55432/closure_stack_customer?sslmode=disable'
if('TestStackOwnDeliveryShare' -match $Run){
 Push-Location (Join-Path $taskRoot 'services/vendor-service')
 try{
  $taskLog=Join-Path $taskEvidence 'TestStackOwnDeliveryShare.log'
  & go test ./internal/features/fulfilmentgroup/ -run '^TestStackOwnDeliveryShare$' -count=1 -v -timeout 10m 2>&1 | Tee-Object -FilePath $taskLog | Select-Object -Last 20
  if($LASTEXITCODE -ne 0){ Write-Output 'VENDOR STACK E2E FAILED' }
 } finally { Pop-Location }
}
