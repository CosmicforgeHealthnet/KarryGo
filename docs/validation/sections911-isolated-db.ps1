param([string]$RunPrefix='closure',[string]$ServiceFilter='',[string[]]$Packages=@(),[string]$Template='')
# Native tools report through $LASTEXITCODE (checked explicitly below);
# 'Stop' would make Windows PowerShell treat any stderr line as fatal.
$ErrorActionPreference = 'Continue'
$taskRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$taskEvidence = Join-Path $taskRoot ('docs/validation/sections911-results-'+$RunPrefix)
New-Item -ItemType Directory -Path $taskEvidence -Force | Out-Null
$taskServices = @(
 @{Path='services/customer-service'; Key='customer'; Env='CUSTOMER_TEST_DATABASE_URL'; Setup=@('test','./internal/database','-run','^TestClosureApplyOwningMigrations$','-count=1')},
 @{Path='services/driver-taxi-service'; Key='taxi'; Env='TAXI_TEST_DATABASE_URL'; Setup=@('test','./internal/database','-run','^TestApplyMigrationsCleanAndReplay$','-count=1')},
 @{Path='services/driver-dispatch-delivery-service'; Key='dispatch'; Env='DISPATCH_TEST_DATABASE_URL'; Setup=@('run','./cmd/test-migrate')},
 @{Path='services/driver-hauling-service'; Key='hauling'; Env='HAULING_TEST_DATABASE_URL'; Setup=@('test','./internal/database','-run','^TestApplyMigrationsTracksAndReplays$','-count=1')},
 @{Path='services/vendor-service'; Key='vendor'; Env='VENDOR_SERVICE_TEST_DATABASE_URL'; Setup=@('test','./internal/database','-run','^TestClosureApplyOwningMigrations$','-count=1')},
 @{Path='shared/go'; Key='shared'; Env='MOVEMENT_TEST_DATABASE_URL'; Setup=@()}
)
$taskResults = @()
foreach ($taskService in $taskServices) {
 if($ServiceFilter -and $taskService.Key -ne $ServiceFilter){continue}
 Push-Location (Join-Path $taskRoot $taskService.Path)
 try {
  $taskTemplate = $RunPrefix+'_tpl_' + $taskService.Key
  # -Template reuses an already-migrated template (reruns of named packages).
  if($Template){ $taskTemplate=$Template }
  if(-not $Template){
  & docker exec karry-customer-sections911-postgres createdb -U karry_test $taskTemplate 2>&1 | Out-File (Join-Path $taskEvidence ($taskService.Key+'-setup.log'))
  if ($LASTEXITCODE -ne 0) { throw 'Template database already exists or could not be created; choose a fresh run prefix.' }
  $taskDsn = 'postgres://karry_test:karry_test@127.0.0.1:55432/'+$taskTemplate+'?sslmode=disable'
  [Environment]::SetEnvironmentVariable($taskService.Env,$taskDsn,'Process')
  $env:CLOSURE_MIGRATION_DATABASE_URL=$taskDsn
  if ($taskService.Setup.Count -gt 0) { & go @($taskService.Setup) 2>&1 | Out-File (Join-Path $taskEvidence ($taskService.Key+'-setup.log')) -Append; if($LASTEXITCODE -ne 0){throw ('Owning migration runner failed: '+$taskService.Key)}; if(Select-String -Path (Join-Path $taskEvidence ($taskService.Key+'-setup.log')) -Pattern 'no tests to run' -Quiet){throw 'Migration test did not execute'} }
  }
  $taskPackages = @(& go list ./...)
  $taskTestFiles = $null
  $taskIndex = 0
  foreach ($taskPackage in $taskPackages) {
   $taskIndex++
   if($Packages.Count -gt 0 -and $Packages -notcontains $taskPackage){continue}
   # Several certification suites refuse any database whose name lacks "test".
   $taskDb=$RunPrefix+'_test_'+$taskService.Key+'_'+$taskIndex
   & docker exec karry-customer-sections911-postgres createdb -U karry_test -T $taskTemplate $taskDb 2>&1 | Out-Null
   if($LASTEXITCODE -ne 0){throw 'Disposable package database clone failed'}
   $taskDsn='postgres://karry_test:karry_test@127.0.0.1:55432/'+$taskDb+'?sslmode=disable'
   # Discover every package's existing database/Redis test-variable aliases.
   $taskEnvNames = @('TEST_DATABASE_URL',$taskService.Env,'CLOSURE_MIGRATION_DATABASE_URL')
   $taskRedisNames = @('JOURNEY_TEST_REDIS_ADDR')
   if(-not $taskTestFiles){ $taskTestFiles = @(Get-ChildItem -Path $PWD -Recurse -File -Filter '*_test.go' | ForEach-Object FullName) }
   foreach($taskFile in $taskTestFiles) {
    $taskContent=[IO.File]::ReadAllText($taskFile)
    foreach($taskMatch in [regex]::Matches($taskContent,'os.Getenv\("([A-Z0-9_]+)"\)')) {
     $taskName=$taskMatch.Groups[1].Value
     if($taskName -match 'TEST.*DATABASE_URL|TEST.*DB_URL'){$taskEnvNames += $taskName}
     if($taskName -match 'TEST.*REDIS_ADDR'){$taskRedisNames += $taskName}
    }
   }
   foreach($taskName in ($taskEnvNames|Select-Object -Unique)){[Environment]::SetEnvironmentVariable($taskName,$taskDsn,'Process')}
   foreach($taskName in ($taskRedisNames|Select-Object -Unique)){[Environment]::SetEnvironmentVariable($taskName,'127.0.0.1:56379','Process')}
   $taskLog=Join-Path $taskEvidence ($taskService.Key+'-'+$taskIndex+'.log')
   & go test $taskPackage -json -count=1 -p 1 -timeout 12m 2>&1 | Out-File $taskLog -Encoding utf8
   $taskExit=$LASTEXITCODE
   $taskEvents=@(Get-Content $taskLog | ForEach-Object {try{$_|ConvertFrom-Json}catch{}})
   $taskPass=@($taskEvents|Where-Object {$_.Action -eq 'pass' -and $_.Test}).Count
   $taskFail=@($taskEvents|Where-Object {$_.Action -eq 'fail' -and $_.Test}).Count
   $taskSkip=@($taskEvents|Where-Object {$_.Action -eq 'skip' -and $_.Test}).Count
   $taskStatus=if($taskExit -ne 0){'FAIL'}elseif($taskPass -eq 0){'SKIPPED'}else{'PASS'}
   $taskResults += [pscustomobject]@{Service=$taskService.Key;Package=$taskPackage;Status=$taskStatus;Pass=$taskPass;Fail=$taskFail;Skipped=$taskSkip;Exit=$taskExit;Evidence=[IO.Path]::GetFileName($taskLog)}
   $taskResults|ConvertTo-Json -Depth 4|Set-Content (Join-Path $taskEvidence 'db-summary.json')
   Write-Output ($taskStatus+' '+$taskPackage+' pass='+$taskPass+' fail='+$taskFail+' skipped='+$taskSkip)
  }
 } finally { Pop-Location }
}
