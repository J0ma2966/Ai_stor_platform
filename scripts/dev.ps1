<#
.SYNOPSIS
  Локальная разработка на Windows без make (аналог Makefile).

.EXAMPLE
  .\scripts\dev.ps1 up            # поднять postgres, redis, minio, api, backup
  .\scripts\dev.ps1 admin root    # создать администратора (спросит пароль)
  .\scripts\dev.ps1 otp           # показать последние SMS-коды из логов (только dev)
  .\scripts\dev.ps1 web           # запустить сайт на http://localhost:3000
  .\scripts\dev.ps1 down          # остановить

  Если PowerShell пишет, что выполнение сценариев отключено:
  powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 up
#>
param(
  [Parameter(Position = 0)]
  [ValidateSet('up', 'down', 'ps', 'logs', 'otp', 'admin', 'web', 'health', 'backup', 'test', 'reset', 'help')]
  [string]$Command = 'help',
  [Parameter(Position = 1)]
  [string]$Arg
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Compose = Join-Path $Root 'deploy/docker-compose.yml'
Set-Location $Root

function Invoke-Compose {
  & docker compose -f $Compose @args
  if ($LASTEXITCODE -ne 0) { throw "docker compose $($args -join ' ') завершился с кодом $LASTEXITCODE" }
}

function Assert-Docker {
  if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Не найден docker. Установите Docker Desktop: https://www.docker.com/products/docker-desktop/'
  }
  # В Windows PowerShell 5.1 перенаправленный stderr при ErrorAction=Stop превращается в исключение.
  $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  & docker info *> $null; $code = $LASTEXITCODE
  $ErrorActionPreference = $prev
  if ($code -ne 0) { throw 'Docker не запущен. Откройте Docker Desktop и дождитесь статуса "Engine running".' }
}

function Assert-Env {
  $envFile = Join-Path $Root '.env'
  if (-not (Test-Path $envFile)) {
    Copy-Item (Join-Path $Root '.env.example') $envFile
    Write-Host 'Создан .env из .env.example (для локальной разработки значения по умолчанию подходят).' -ForegroundColor Yellow
  }
}

function Wait-Ready {
  Write-Host 'Жду готовности API на http://localhost:8080/readyz ...'
  for ($i = 0; $i -lt 60; $i++) {
    try {
      # 127.0.0.1, а не localhost: порты проброшены только на IPv4, а PowerShell 7 сначала пробует ::1 и не успевает за 2 секунды.
      $r = Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 http://127.0.0.1:8080/readyz
      if ($r.StatusCode -eq 200) { Write-Host 'API готов.' -ForegroundColor Green; return }
    } catch { }
    Start-Sleep -Seconds 2
  }
  Write-Warning 'API не ответил за 2 минуты. Посмотрите логи: .\scripts\dev.ps1 logs'
}

switch ($Command) {
  'up' {
    Assert-Docker; Assert-Env
    Invoke-Compose up -d --build
    Wait-Ready
    Write-Host 'MinIO консоль: http://localhost:9001 (minioadmin / minioadmin-dev)'
    Write-Host 'Дальше: .\scripts\dev.ps1 admin root, затем .\scripts\dev.ps1 web'
  }
  'down'   { Assert-Docker; Invoke-Compose down }
  'reset'  {
    Assert-Docker
    $ok = Read-Host 'Удалить контейнеры И все локальные данные (БД, файлы, бэкапы)? Введите yes'
    if ($ok -eq 'yes') { Invoke-Compose down -v } else { Write-Host 'Отменено.' }
  }
  'ps'     { Assert-Docker; Invoke-Compose ps }
  'logs'   { Assert-Docker; if ($Arg) { Invoke-Compose logs -f --tail 200 $Arg } else { Invoke-Compose logs -f --tail 200 api } }
  'otp'    {
    Assert-Docker
    & docker compose -f $Compose logs --tail 500 api | Select-String 'DEV OTP' | Select-Object -Last 5
  }
  'health' { Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/readyz | Select-Object StatusCode, Content }
  'admin'  {
    Assert-Docker
    $login = if ($Arg) { $Arg } else { 'root' }
    $secure = Read-Host "Пароль для администратора '$login' (минимум 10 символов)" -AsSecureString
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
    $env:ADMIN_PASSWORD = $plain
    try { Invoke-Compose exec -e ADMIN_PASSWORD api /api create-admin $login }
    finally { Remove-Item Env:ADMIN_PASSWORD -ErrorAction SilentlyContinue }
  }
  'web'    {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { throw 'Не найден npm. Установите Node.js 20 LTS или новее: https://nodejs.org/' }
    Push-Location (Join-Path $Root 'web')
    try {
      # Пустая или недоустановленная node_modules (прерванный npm ci) тоже требует установки.
      if (-not (Test-Path 'node_modules/.bin/next*')) { & npm ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci завершился с ошибкой' } }
      $env:NEXT_PUBLIC_API_URL = 'http://localhost:8080'
      $env:NEXT_PUBLIC_MEDIA_ORIGIN = 'http://localhost:9000'
      & npm run dev
    } finally { Pop-Location }
  }
  'backup' { Assert-Docker; Invoke-Compose exec backup backup.sh }
  'test'   {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Не найден Go. Установите Go 1.26+: https://go.dev/dl/' }
    if (-not $env:TEST_DATABASE_URL) {
      $env:TEST_DATABASE_URL = 'postgres://boutique:devpassword@localhost:5432/boutique?sslmode=disable'
      Write-Host "TEST_DATABASE_URL не задан, использую Postgres из docker compose." -ForegroundColor Yellow
    }
    Push-Location (Join-Path $Root 'backend')
    try { & go test -count=1 ./...; if ($LASTEXITCODE -ne 0) { throw 'Тесты упали' } } finally { Pop-Location }
  }
  default  { Get-Help $PSCommandPath -Detailed }
}
