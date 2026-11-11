$ErrorActionPreference = "Stop"

$ResourceFile = ".\cmd\tbc\resource.syso"
$VersionInfo = ".\build\windows\versioninfo.json"
$OutputFile = ".\tbc.exe"

try {
    Write-Host "[1/3] Generating Windows resources..." -ForegroundColor Cyan

    goversioninfo -64 -o $ResourceFile $VersionInfo

    if (-not (Test-Path $ResourceFile)) {
        throw "resource.syso was not generated!"
    }

    Write-Host "[2/3] Building tbc.exe..." -ForegroundColor Cyan

    go build -o $OutputFile .\cmd\tbc\

    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed!"
    }

    Write-Host "[3/3] Build completed successfully!" -ForegroundColor Green
}
finally {
    if (Test-Path $ResourceFile) {
        Remove-Item $ResourceFile -Force
        Write-Host "Temporary resource.syso removed." -ForegroundColor Yellow
    }
}

Write-Host "Output: $OutputFile" -ForegroundColor Green
