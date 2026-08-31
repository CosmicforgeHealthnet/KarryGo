param()

$ErrorActionPreference = "Stop"

if (-not $env:VENDOR_SERVICE_TEST_DATABASE_URL) {
    throw "VENDOR_SERVICE_TEST_DATABASE_URL is required."
}

go run ./tools/vendor-test-runner

if ($LASTEXITCODE -ne 0) {
    throw "Vendor isolated regression failed."
}