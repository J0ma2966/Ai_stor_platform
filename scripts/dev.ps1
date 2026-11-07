<#
.SYNOPSIS
  Локальная разработка на Windows без make (аналог Makefile).

.EXAMPLE
  .\scripts\dev.ps1 up            # поднять postgres, redis, minio, api, backup
                                  # (занятые или зарезервированные Windows порты заменит сам, см. .env)
  .\scripts\dev.ps1 admin root    # создать администратора (спросит пароль)
  .\scripts\dev.ps1 demo          # тестовые магазины и товары (спросит пароль для кабинетов магазинов)
  .\scripts\dev.ps1 demo remove   # удалить тестовые магазины и товары
  .\scripts\dev.ps1 otp           # показать последние SMS-коды из логов (только dev)
  .\scripts\dev.ps1 web           # запустить сайт на http://localhost:3000
  .\scripts\dev.ps1 down          # остановить

  Если PowerShell пишет, что выполнение сценариев отключено:
  powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 up
#>
param(
  [Parameter(Position = 0)]
  [ValidateSet('up', 'down', 'ps', 'logs', 'otp', 'admin', 'demo', 'web', 'health', 'backup', 'test', 'reset', 'help')]
  [string]$Command = 'help',
  [Parameter(Position = 1)]
  [string]$Arg
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Compose = Join-Path $Root 'deploy/docker-compose.yml'
$EnvFile = Join-Path $Root '.env'
Set-Location $Root

# Порты на этом компьютере и значения по умолчанию. Меняются в .env (см. .env.example).
$DefaultPorts = [ordered]@{
  POSTGRES_HOST_PORT      = 5432
  REDIS_HOST_PORT         = 6379
  MINIO_HOST_PORT         = 9000
  MINIO_CONSOLE_HOST_PORT = 9001
  API_HOST_PORT           = 8080
  METRICS_HOST_PORT       = 9090
  WEB_PORT                = 3000
}

function Invoke-Compose {
  # --env-file: иначе compose ищет .env в deploy/ и не видит порты из корневого .env
  if (Test-Path $EnvFile) { & docker compose -f $Compose --env-file $EnvFile @args }
  else { & docker compose -f $Compose @args }
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

# .env читается и пишется как UTF-8 без BOM: Get-Content в PowerShell 5.1 портит кириллицу,
# а BOM ломает первую переменную для docker compose.
function Read-DotEnv {
  $values = @{}
  if (Test-Path $EnvFile) {
    foreach ($line in [IO.File]::ReadAllLines($EnvFile)) {
      if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') { $values[$Matches[1]] = $Matches[2].Trim() }
    }
  }
  return $values
}

function Set-DotEnv([string]$Name, [string]$Value) {
  $lines = New-Object System.Collections.Generic.List[string]
  if (Test-Path $EnvFile) { $lines.AddRange([string[]][IO.File]::ReadAllLines($EnvFile)) }
  $found = $false
  for ($i = 0; $i -lt $lines.Count; $i++) {
    if ($lines[$i] -match "^\s*$Name\s*=") { $lines[$i] = "$Name=$Value"; $found = $true }
  }
  if (-not $found) { $lines.Add("$Name=$Value") }
  [IO.File]::WriteAllLines($EnvFile, $lines.ToArray(), (New-Object System.Text.UTF8Encoding $false))
}

# Как и docker compose: переменная окружения терминала важнее значения из .env.
function Get-Port([string]$Name) {
  $v = [Environment]::GetEnvironmentVariable($Name)
  if (-not $v) { $v = (Read-DotEnv)[$Name] }
  if ($v -match '^\d+$') { return [int]$v }
  return [int]$DefaultPorts[$Name]
}

# Диапазоны, которые Windows (Hyper-V, WSL, Docker Desktop) зарезервировала под себя.
# Порт из такого диапазона не открыть даже свободным: «access a socket in a way forbidden».
function Get-ExcludedRanges {
  if ($env:OS -ne 'Windows_NT') { return @() }
  $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  $out = & netsh interface ipv4 show excludedportrange protocol=tcp 2>$null
  $ErrorActionPreference = $prev
  $ranges = @()
  foreach ($line in $out) {
    if ($line -match '^\s*(\d+)\s+(\d+)') { $ranges += [pscustomobject]@{ Start = [int]$Matches[1]; End = [int]$Matches[2] } }
  }
  return $ranges
}

function Test-Excluded([int]$Port, $Ranges) {
  foreach ($r in $Ranges) { if ($Port -ge $r.Start -and $Port -le $r.End) { return $true } }
  return $false
}

function Test-PortFree([int]$Port) {
  # Слушающий сокет на 0.0.0.0 не мешает занять 127.0.0.1 на Windows, поэтому спрашиваем и систему.
  if (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue) {
    if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) { return $false }
  }
  try {
    $l = New-Object System.Net.Sockets.TcpListener([Net.IPAddress]::Loopback, $Port)
    $l.Start(); $l.Stop(); return $true
  } catch { return $false }
}

function Find-FreePort([int]$From, $Ranges, $Taken) {
  for ($p = $From; $p -le 65535; $p++) {
    if (-not (Test-Excluded $p $Ranges) -and -not $Taken.ContainsKey($p) -and (Test-PortFree $p)) { return $p }
  }
  throw "Не нашёл свободный порт начиная с $From"
}

# Перед up: если порт зарезервирован Windows или занят, берём свободный и записываем его в .env.
function Resolve-Ports {
  $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  # Порты, которые уже держат наши собственные запущенные контейнеры (повторный up).
  $ours = (& docker compose -f $Compose --env-file $EnvFile ps --format '{{.Ports}}' 2>$null) -join ' '
  $ErrorActionPreference = $prev
  $ranges = Get-ExcludedRanges
  $taken = @{}
  foreach ($name in @($DefaultPorts.Keys)) {
    $port = Get-Port $name
    $problem = $null
    if (Test-Excluded $port $ranges) { $problem = 'зарезервирован Windows' }
    elseif ($taken.ContainsKey($port)) { $problem = 'уже отдан другому сервису' }
    # Сайт мог быть уже запущен отдельно (dev.ps1 web), его занятость здесь не проверяем.
    elseif ($name -ne 'WEB_PORT' -and $ours -notlike "*:$port->*" -and -not (Test-PortFree $port)) { $problem = 'занят другой программой' }
    if ($problem) {
      if ([Environment]::GetEnvironmentVariable($name)) {
        throw "Порт $port ($name) $problem. Уберите переменную `$env:$name или задайте другой порт."
      }
      $new = Find-FreePort ([int]$DefaultPorts[$name] + 10000) $ranges $taken
      Write-Host "Порт $port ($name) $problem, использую $new и записываю в .env." -ForegroundColor Yellow
      Set-DotEnv $name $new
      if ($name -eq 'WEB_PORT') { Update-WebOrigin $port $new }
      $port = $new
    }
    $taken[$port] = $true
  }
}

# Сайт переехал на другой порт: API должен разрешить новый адрес (CORS) и возвращать на него после оплаты.
function Update-WebOrigin([int]$Old, [int]$New) {
  $cfg = Read-DotEnv
  $origin = "http://localhost:$New"
  $cors = $cfg['CORS_ORIGINS']
  if (-not $cors) { $cors = $origin }
  elseif (($cors -split ',') -notcontains $origin) { $cors = "$cors,$origin" }
  Set-DotEnv 'CORS_ORIGINS' $cors
  if ($cfg['PAYMENT_RETURN_URL']) {
    Set-DotEnv 'PAYMENT_RETURN_URL' ($cfg['PAYMENT_RETURN_URL'].Replace("http://localhost:$Old/", "$origin/"))
  }
}

function Get-ApiUrl { "http://localhost:$(Get-Port 'API_HOST_PORT')" }

# Для проверок из скрипта: localhost на Windows сначала идёт на IPv6 (::1), где Docker не слушает,
# и отказ там приходит только через ~2 секунды. Поэтому проверяем напрямую по IPv4.
function Get-ApiProbeUrl { "http://127.0.0.1:$(Get-Port 'API_HOST_PORT')" }

function Wait-Ready {
  $url = "$(Get-ApiProbeUrl)/readyz"
  Write-Host "Жду готовности API на $url ..."
  for ($i = 0; $i -lt 60; $i++) {
    try {
      $r = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 $url
      if ($r.StatusCode -eq 200) { Write-Host 'API готов.' -ForegroundColor Green; return }
    } catch { }
    Start-Sleep -Seconds 2
  }
  Write-Warning 'API не ответил за 2 минуты. Посмотрите логи: .\scripts\dev.ps1 logs'
}

switch ($Command) {
  'up' {
    Assert-Docker; Assert-Env; Resolve-Ports
    Invoke-Compose up -d --build
    Wait-Ready
    Write-Host "API: $(Get-ApiUrl)"
    Write-Host "MinIO консоль: http://localhost:$(Get-Port 'MINIO_CONSOLE_HOST_PORT') (minioadmin / minioadmin-dev)"
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
    Invoke-Compose logs --tail 500 api | Select-String 'DEV OTP' | Select-Object -Last 5
  }
  'health' { Invoke-WebRequest -UseBasicParsing "$(Get-ApiProbeUrl)/readyz" | Select-Object StatusCode, Content }
  'admin'  {
    Assert-Docker
    $login = if ($Arg) { $Arg } else { 'root' }
    $secure = Read-Host "Пароль для администратора '$login' (минимум 10 символов)" -AsSecureString
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
    $env:ADMIN_PASSWORD = $plain
    try { Invoke-Compose exec -e ADMIN_PASSWORD api /api create-admin $login }
    finally { Remove-Item Env:ADMIN_PASSWORD -ErrorAction SilentlyContinue }
  }
  'demo'   {
    Assert-Docker
    if ($Arg -eq 'remove') { Invoke-Compose exec api /api demo-data remove; break }
    $secure = Read-Host 'Пароль для входа в кабинеты тестовых магазинов (минимум 10 символов)' -AsSecureString
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure))
    $env:DEMO_STORE_PASSWORD = $plain
    try { Invoke-Compose exec -e DEMO_STORE_PASSWORD api /api demo-data }
    finally { Remove-Item Env:DEMO_STORE_PASSWORD -ErrorAction SilentlyContinue }
  }
  'web'    {
    if (-not (Get-Command npm -ErrorAction SilentlyContinue)) { throw 'Не найден npm. Установите Node.js 20 LTS или новее: https://nodejs.org/' }
    Push-Location (Join-Path $Root 'web')
    try {
      # Пустая или недоустановленная node_modules (прерванный npm ci) тоже требует установки.
      if (-not (Test-Path 'node_modules/.bin/next*')) { & npm ci; if ($LASTEXITCODE -ne 0) { throw 'npm ci завершился с ошибкой' } }
      $env:NEXT_PUBLIC_API_URL = Get-ApiUrl
      # Запросы со стороны сервера Next.js: тоже мимо IPv6 localhost
      $env:API_INTERNAL_URL = Get-ApiProbeUrl
      # Фото отдаются с адреса из DEV_PUBLIC_HOST (IP компьютера, если проверяете с телефона), иначе localhost
      $mediaHost = (Read-DotEnv)['DEV_PUBLIC_HOST']
      if (-not $mediaHost) { $mediaHost = 'localhost' }
      $env:NEXT_PUBLIC_MEDIA_ORIGIN = "http://${mediaHost}:$(Get-Port 'MINIO_HOST_PORT')"
      $webPort = Get-Port 'WEB_PORT'
      if (-not (Test-PortFree $webPort)) {
        throw "Порт $webPort для сайта занят или зарезервирован Windows. Выполните .\scripts\dev.ps1 up (подберёт другой) или задайте WEB_PORT в .env."
      }
      Write-Host "Сайт: http://localhost:$webPort/ru" -ForegroundColor Green
      & npm run dev -- -p $webPort
    } finally { Pop-Location }
  }
  'backup' { Assert-Docker; Invoke-Compose exec backup backup.sh }
  'test'   {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Не найден Go. Установите Go 1.26+: https://go.dev/dl/' }
    if (-not $env:TEST_DATABASE_URL) {
      $env:TEST_DATABASE_URL = "postgres://boutique:devpassword@localhost:$(Get-Port 'POSTGRES_HOST_PORT')/boutique?sslmode=disable"
      Write-Host "TEST_DATABASE_URL не задан, использую Postgres из docker compose." -ForegroundColor Yellow
    }
    Push-Location (Join-Path $Root 'backend')
    try { & go test -count=1 ./...; if ($LASTEXITCODE -ne 0) { throw 'Тесты упали' } } finally { Pop-Location }
  }
  default  { Get-Help $PSCommandPath -Detailed }
}
