# Helpers for the setup's end-to-end cases; each installer step dot-sources this file.

$SetupExe = "$env:GITHUB_WORKSPACE\Installer\Output\SemiBase-Setup.exe"
$LogDir = "$env:ProgramFiles\SemiBase Setup\Logs"
$ToolExe = "$env:ProgramFiles\SemiBase Setup\semibase.exe"
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

# Setup is a GUI program, so the call operator would not wait for it. ProcessStartInfo passes the
# argument string as written, so a " reaches Setup; Start-Process would rebuild the string.
function Invoke-Setup([string] $arguments = '') {
  $start = [Diagnostics.ProcessStartInfo]::new($SetupExe, "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART $arguments")
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

# A refusal is exit 7 with its reason as the last line of the newest setup log: LogRefusal
# appends to a log of the same second, so earlier lines may belong to an earlier run
function Assert-Refused($run, [string] $want) {
  if ($run.ExitCode -ne 7) { Fail "want exit 7, a refusal from PrepareToInstall, with '$want', got $($run.ExitCode)" }
  $log = $run.Logs | Where-Object { $_.Name -like 'setup-*' } | Select-Object -Last 1
  $last = if ($log) { Get-Content $log.FullName | Select-Object -Last 1 }
  Write-Host "refusal: $last"
  if (-not "$last".Contains($want)) { Fail "the setup log does not end with '$want'" }
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

# Returns the trimmed output of one statement run as postgres
function Invoke-Super([string] $password, [string] $sql) {
  $env:PGPASSWORD = $password
  $out = & "$env:PGBIN\psql.exe" -w -h 127.0.0.1 -U postgres -d postgres -qtAc $sql
  if ($LASTEXITCODE -ne 0) { Fail "psql as postgres failed on: $sql" }
  "$out".Trim()
}
