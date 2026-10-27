# Helpers for the setup's end-to-end cases; each installer step dot-sources this file.

$SetupExe = "$env:GITHUB_WORKSPACE\Installer\Output\SemiBase-Setup.exe"
$AppDir = "$env:ProgramFiles\SemiBase"
$LogDir = "$AppDir\Logs"
$ToolExe = "$AppDir\semibase.exe"
$TokenFile = "$env:RUNNER_TEMP\case-passwords.txt"
# The registry key and service of the runner's 17 and of the one the setup installs
$PgInstallation = 'postgresql-x64-17'
$PgService = 'postgresql-x64-17'

function Fail([string] $message) {
  Write-Host "::error::$message"
  exit 1
}

function New-Token {
  -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 24 | ForEach-Object { [char] $_ })
}

# Setup is a GUI program, so the call operator would not wait for it. /LANG=english keeps the
# refusal texts the cases assert.
function Invoke-Setup {
  $start = [Diagnostics.ProcessStartInfo]::new($SetupExe, '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LANG=english')
  $start.UseShellExecute = $false
  $since = Get-Date
  $setup = [Diagnostics.Process]::Start($start)
  $setup.WaitForExit()
  $logs = @(Get-ChildItem "$LogDir\*.log" -ErrorAction SilentlyContinue |
      Where-Object { $_.LastWriteTime -ge $since } | Sort-Object LastWriteTime)
  [pscustomobject]@{ ExitCode = $setup.ExitCode; Logs = $logs }
}

# The setup and site logs carry no password by design; the redaction keeps a regression out of
# the public job log
function Write-Redacted([string] $text, [string[]] $tokens = @()) {
  foreach ($token in $tokens) { $text = $text.Replace($token, '<REDACTED>') }
  Write-Host $text
}

function Write-Logs($run, [string[]] $tokens = @()) {
  foreach ($log in $run.Logs) {
    Write-Host "--- $($log.Name)"
    Write-Redacted "$(Get-Content -Raw $log.FullName)" $tokens
  }
}

function Get-SiteLog($run) {
  $site = $run.Logs | Where-Object { $_.Name -like 'site-*' } | Select-Object -Last 1
  if (-not $site) { Fail 'the run wrote no site log' }
  "$(Get-Content -Raw $site.FullName)"
}

# A refusal is exit 7 with its reason as the last line of the newest setup log and its technical
# detail, when it has one, on the line before: LogRefusal appends to a log of the same second, so
# earlier lines may belong to an earlier run
function Assert-Refused($run, [string] $want, [string] $detail = '') {
  if ($run.ExitCode -ne 7) { Fail "want exit 7, a refusal from PrepareToInstall, with '$want', got $($run.ExitCode)" }
  $log = $run.Logs | Where-Object { $_.Name -like 'setup-*' } | Select-Object -Last 1
  # @() outside the if: assigning an if statement unrolls a one-element array into its string
  $tail = @(if ($log) { Get-Content $log.FullName | Select-Object -Last 2 })
  $last = if ($tail.Count -gt 0) { $tail[-1] }
  $before = if ($tail.Count -eq 2) { $tail[0] }
  Write-Host "refusal: $last"
  if ($detail) { Write-Host "detail: $before" }
  if (-not "$last".Contains($want)) { Fail "the setup log does not end with '$want'" }
  if ($detail -and -not "$before".Contains($detail)) { Fail "the line before the refusal does not hold '$detail'" }
}

function Assert-NoPostgresService {
  if (Get-Service postgresql* -ErrorAction SilentlyContinue) { Fail 'a refused run left a postgresql* service' }
}

function Assert-NothingRegistered {
  foreach ($key in 'Installations', 'Services') {
    $path = "HKLM:\SOFTWARE\PostgreSQL\$key"
    if ((Test-Path $path) -and @(Get-ChildItem $path).Count -ne 0) { Fail "$path is not empty" }
  }
}

function Assert-Login([string] $role, [string] $password, [string] $database = 'scada_archive') {
  $env:PGPASSWORD = $password
  & "$env:PGBIN\psql.exe" -w -h 127.0.0.1 -U $role -d $database -tAc 'SELECT 1' | Out-Null
  if ($LASTEXITCODE -ne 0) { Fail "$role cannot log in to $database with the expected password" }
}

function Invoke-Super([string] $password, [string] $sql) {
  $env:PGPASSWORD = $password
  $out = & "$env:PGBIN\psql.exe" -w -h 127.0.0.1 -U postgres -d postgres -qtAc $sql
  if ($LASTEXITCODE -ne 0) { Fail "psql as postgres failed on: $sql" }
  "$out".Trim()
}

# The uninstaller exits before it removes anything, and a copy of it in %TEMP% does the work
# (docs/architecture/installer.md#the-uninstaller). The empty Installations and Services keys
# it leaves stay, as on an installation machine.
function Start-PostgreSQLRemoval {
  $base = (Get-ItemProperty "HKLM:\SOFTWARE\PostgreSQL\Installations\$PgInstallation").'Base Directory'
  $uninstaller = Start-Process -FilePath "$base\uninstall-postgresql.exe" `
    -ArgumentList '--mode', 'unattended', '--unattendedmodeui', 'none' -PassThru
  # A process object whose handle was never read reports no ExitCode after it exits
  $null = $uninstaller.Handle
  $uninstaller
}

function Get-UninstallerCopy {
  @(Get-CimInstance Win32_Process | Where-Object { $_.Name -like '_uninstall*.000' })
}

function Wait-PostgreSQLRemoval($uninstaller) {
  $uninstaller.WaitForExit()
  if ($uninstaller.ExitCode -ne 0) { Fail "uninstall-postgresql.exe exited $($uninstaller.ExitCode)" }
  $deadline = (Get-Date).AddMinutes(5)
  while ((Get-UninstallerCopy).Count -ne 0) {
    if ((Get-Date) -gt $deadline) { Fail 'the uninstaller copy still runs after 5 minutes' }
    Start-Sleep -Seconds 1
  }
  $last = Get-Content "$env:TEMP\uninstall-postgresql.log" | Where-Object { $_.Trim() } | Select-Object -Last 1
  if ("$last" -notlike '*Exiting with code 0*') { Fail "the uninstall log ends with '$last' (want Exiting with code 0)" }
  foreach ($path in 'C:\PostgreSQL\17\data', 'C:\Program Files\PostgreSQL') {
    if (Test-Path $path) { Remove-Item -Recurse -Force $path }
  }
  reg.exe query HKLM\SOFTWARE\PostgreSQL /s
  Assert-NothingRegistered
  $services = @(Get-Service postgresql* -ErrorAction SilentlyContinue)
  if ($services.Count -ne 0) { Fail "left behind: $($services.Count) postgresql* services" }
  $client = [Net.Sockets.TcpClient]::new()
  try { $open = $client.ConnectAsync('127.0.0.1', 5432).Wait(2000) } catch { $open = $false } finally { $client.Close() }
  if ($open) { Fail '127.0.0.1:5432 still accepts TCP' }
}

function Remove-PostgreSQL {
  Wait-PostgreSQLRemoval (Start-PostgreSQLRemoval)
}
