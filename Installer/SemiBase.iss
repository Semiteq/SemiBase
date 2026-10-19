; SemiBase Inno Setup installer script
; Build with: iscc.exe /DAppVersion=1.2.3 /DFileVersion=1.2.3 /DSemibaseExe=<path to semibase.exe> Installer\SemiBase.iss
; Run `bash Installer/fetch-postgresql.sh` first; it puts the EDB installer into Installer\payload\.

#ifndef AppVersion
  #error AppVersion must be defined. Pass it as: iscc.exe /DAppVersion=1.2.3 SemiBase.iss
#endif
#ifndef FileVersion
  #error FileVersion must be defined, digits and dots only. Pass it as: iscc.exe /DFileVersion=1.2.3 SemiBase.iss
#endif
#ifndef SemibaseExe
  #error SemibaseExe must be defined. Pass it as: iscc.exe /DSemibaseExe=C:\path\to\semibase.exe SemiBase.iss
#endif

#define AppName      "SemiBase"
#define AppPublisher "Inc Semiteq"
#define AppId        "{{5AFFFB1F-3C84-45EB-800A-67812EBAE460}"

; The file name carries the version pinned in fetch-postgresql.sh; a new pin changes both
#define PgInstaller  "postgresql-17.11-5-windows-x64.exe"

#define LogRoot      "C:\DISTR\Logs\SemiBase"

; The PostgreSQL 17 install this setup creates: its registry key, service, port and data directory
#define PgInstallation "postgresql-x64-17"
#define PgService      "postgresql-x64-17"
#define PgPort         "5432"
#define PgDataDir      "C:\Program Files\PostgreSQL\17\data"

[Setup]
AppId={#AppId}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppVerName={#AppName} {#AppVersion}
VersionInfoVersion={#FileVersion}

ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes

OutputDir=Output
OutputBaseFilename=SemiBase-Setup

Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes

WizardStyle=modern

; Program Files, C:\DISTR and the PostgreSQL service all need admin rights
PrivilegesRequired=admin

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Dirs]
Name: "{#LogRoot}"; Flags: uninsneveruninstall

[Files]
Source: "{#SemibaseExe}"; DestDir: "{app}"; DestName: "semibase.exe"; Flags: ignoreversion

; Stored as is, so extracting it in [Code] copies it instead of decompressing it
Source: "payload\{#PgInstaller}"; Flags: dontcopy nocompression

[Code]
function SetEnvironmentVariable(lpName, lpValue: String): BOOL;
  external 'SetEnvironmentVariableW@kernel32.dll stdcall';
function GetTickCount: Cardinal;
  external 'GetTickCount@kernel32.dll stdcall';

function CommandLineParameter(const Name: String): String;
var
  Prefix: String;
  I: Integer;
begin
  Result := '';
  Prefix := '/' + Name + '=';
  for I := 1 to ParamCount do
    if CompareText(Copy(ParamStr(I), 1, Length(Prefix)), Prefix) = 0 then
    begin
      Result := Copy(ParamStr(I), Length(Prefix) + 1, Length(ParamStr(I)));
      Exit;
    end;
end;

// Setup's command-line parser drops every ", so a value typed with one arrives changed
function ParameterRefusal(const Name, VariableName: String): String;
var
  Value, Prefix, Tail: String;
  ValueStart: Integer;
begin
  Result := '';
  Value := CommandLineParameter(Name);
  if Value = '' then
    Exit;
  Prefix := '/' + Name + '=';
  Tail := GetCmdTail;
  ValueStart := Pos(Uppercase(Prefix), Uppercase(Tail)) + Length(Prefix);
  if Copy(Tail, ValueStart, Length(Value)) <> Value then
    Result := 'The /' + Name + ' value does not reach Setup as typed: Setup drops every " from ' +
      'its command line. Put the quotes around the whole parameter, or pass the password ' +
      'through ' + VariableName + '.';
end;

function ResolvePassword(const ParameterName, VariableName: String): String;
begin
  Result := CommandLineParameter(ParameterName);
  if Result = '' then
    Result := GetEnv(VariableName);
end;

function IsPrintableAscii(const Value: String): Boolean;
var
  I: Integer;
begin
  Result := True;
  for I := 1 to Length(Value) do
    if (Ord(Value[I]) < 32) or (Ord(Value[I]) > 126) then
    begin
      Result := False;
      Exit;
    end;
end;

function PasswordRefusal(const SuperPassword, WriterPassword: String): String;
begin
  Result := '';
  if SuperPassword = '' then
    Result := 'No superuser password: pass /SUPERPASSWORD=<password> or set SEMIBASE_SUPER_PASSWORD.'
  else if not IsPrintableAscii(SuperPassword) then
    Result := 'The superuser password must be printable ASCII: the PostgreSQL installer stores it ' +
      'in the ANSI code page, and semibase sends it in UTF-8.'
  else if (Pos('"', SuperPassword) > 0) or (SuperPassword[Length(SuperPassword)] = '\') then
    Result := 'The superuser password must not contain " or end in \: either one breaks the ' +
      'PostgreSQL installer''s command line.'
  else if not IsPrintableAscii(WriterPassword) then
    Result := 'The scada_writer password (/WRITERPASSWORD or SEMIBASE_WRITER_PASSWORD) must be ' +
      'printable ASCII.';
end;

const
  InstallationsKey = 'SOFTWARE\PostgreSQL\Installations';
  ServicesKey = 'SOFTWARE\PostgreSQL\Services';
  RecoveryHint = ' See the recovery section of docs/deployment.md.';
  PortWaitMilliseconds = 60000;
  ErrorServiceAlreadyRunning = 1056;

type
  TEngine = record
    Fresh: Boolean;
    ServiceName: String;
    Port: Cardinal;
    SuperUser: String;
  end;

function LogPath(const Prefix: String): String;
begin
  Result := '{#LogRoot}\' + Prefix + '-' + GetDateTimeString('yyyymmdd-hhnnss', #0, #0) + '.log';
end;

// Under /VERYSILENT the returned refusal reaches no one, and /LOG would record the passwords
procedure LogRefusal(const Message: String);
var
  Lines: TArrayOfString;
begin
  SetArrayLength(Lines, 1);
  Lines[0] := Message;
  ForceDirectories('{#LogRoot}');
  SaveStringsToUTF8FileWithoutBOM(LogPath('setup'), Lines, True);
end;

// -1 when sc.exe did not run
function ScExitCode(const Arguments: String): Integer;
var
  ExitCode: Integer;
begin
  if Exec(ExpandConstant('{sys}\sc.exe'), Arguments, '', SW_HIDE, ewWaitUntilTerminated,
    ExitCode) then
    Result := ExitCode
  else
    Result := -1;
end;

function ServiceExists(const Name: String): Boolean;
begin
  Result := ScExitCode('query "' + Name + '"') = 0;
end;

// Pascal Script has no sockets. Returns False when the probe itself did not run.
function ProbePort(Port: Cardinal; var Accepts: Boolean): Boolean;
var
  ExitCode: Integer;
begin
  Result := Exec(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'),
    '-NoProfile -NonInteractive -Command "$c = New-Object Net.Sockets.TcpClient; ' +
    'try { $open = $c.ConnectAsync(''127.0.0.1'', ' + IntToStr(Port) + ').Wait(2000) } ' +
    'catch { $open = $false }; $c.Close(); if ($open) { exit 0 } else { exit 2 }"',
    '', SW_HIDE, ewWaitUntilTerminated, ExitCode) and ((ExitCode = 0) or (ExitCode = 2));
  Accepts := ExitCode = 0;
end;

function IsNonEmptyDirectory(const Path: String): Boolean;
var
  Entry: TFindRec;
begin
  Result := False;
  if FindFirst(AddBackslash(Path) + '*', Entry) then
    try
      repeat
        Result := (Entry.Name <> '.') and (Entry.Name <> '..');
      until Result or not FindNext(Entry);
    finally
      FindClose(Entry);
    end;
end;

// The 17 key exists: it is used only when it is a complete install with a service
function RegisteredEngineRefusal(var Engine: TEngine): String;
var
  Key, Version, SuperUser, DataDirectory, BaseDirectory, ServiceName, Missing: String;
  Port: Cardinal;
begin
  Result := '';
  Key := InstallationsKey + '\{#PgInstallation}';
  RegQueryStringValue(HKLM64, Key, 'Version', Version);
  RegQueryStringValue(HKLM64, Key, 'Super User', SuperUser);
  RegQueryStringValue(HKLM64, Key, 'Data Directory', DataDirectory);
  RegQueryStringValue(HKLM64, Key, 'Service ID', ServiceName);

  if Copy(Version, 1, 3) <> '17.' then
    Missing := 'its Version is ''' + Version + ''''
  else if SuperUser = '' then
    Missing := 'it has no Super User'
  else if DataDirectory = '' then
    Missing := 'it has no Data Directory'
  else if not FileExists(AddBackslash(DataDirectory) + 'PG_VERSION') then
    Missing := AddBackslash(DataDirectory) + 'PG_VERSION does not exist';
  if Missing <> '' then
  begin
    if not RegQueryStringValue(HKLM64, Key, 'Base Directory', BaseDirectory) then
      BaseDirectory := '<Base Directory>';
    Result := 'HKLM\' + Key + ' is not a complete PostgreSQL 17 installation: ' + Missing +
      '. A failed PostgreSQL install leaves this; remove it with ' + BaseDirectory +
      '\uninstall-postgresql.exe, then run Setup again.' + RecoveryHint;
    Exit;
  end;

  if ServiceName = '' then
    Result := 'HKLM\' + Key + ' names no service. Setup does not repair an orphaned ' +
      'registration.' + RecoveryHint
  else if not ServiceExists(ServiceName) then
    Result := 'HKLM\' + Key + ' names service ' + ServiceName + ', which does not exist. Setup ' +
      'does not repair an orphaned registration.' + RecoveryHint
  else if not RegQueryDWordValue(HKLM64, ServicesKey + '\' + ServiceName, 'Port', Port) then
    Result := 'HKLM\' + ServicesKey + '\' + ServiceName + ' has no Port value. Setup does not ' +
      'repair an orphaned registration.' + RecoveryHint;
  if Result <> '' then
    Exit;

  Engine.ServiceName := ServiceName;
  Engine.Port := Port;
  Engine.SuperUser := SuperUser;
end;

// The 17 key does not exist, and another major may be registered
function OtherMajorRefusal: String;
var
  Names: TArrayOfString;
  I: Integer;
  Version, ServiceName, PortText, Registered: String;
  Port: Cardinal;
begin
  Result := '';
  if not RegGetSubkeyNames(HKLM64, InstallationsKey, Names) then
    Exit;
  for I := 0 to GetArrayLength(Names) - 1 do
  begin
    Version := '';
    ServiceName := '';
    RegQueryStringValue(HKLM64, InstallationsKey + '\' + Names[I], 'Version', Version);
    RegQueryStringValue(HKLM64, InstallationsKey + '\' + Names[I], 'Service ID', ServiceName);
    if RegQueryDWordValue(HKLM64, ServicesKey + '\' + ServiceName, 'Port', Port) then
      PortText := 'port ' + IntToStr(Port)
    else
      PortText := 'port unknown';
    if Registered <> '' then
      Registered := Registered + '; ';
    Registered := Registered + 'HKLM\' + InstallationsKey + '\' + Names[I] + ' (version ' +
      Version + ', service ' + ServiceName + ', ' + PortText + ')';
  end;
  if Registered <> '' then
    Result := 'PostgreSQL 17 is not registered, and another major is: ' + Registered +
      '. Setup provisions PostgreSQL 17 only.' + RecoveryHint;
end;

// Nothing is registered: a fresh install must not meet a server or data it does not know
function FreshInstallRefusal(const WriterPassword: String): String;
var
  Accepts: Boolean;
begin
  Result := '';
  if not ProbePort({#PgPort}, Accepts) then
    Result := 'Setup could not probe 127.0.0.1:{#PgPort} through powershell.exe.' + RecoveryHint
  else if Accepts then
    Result := 'No PostgreSQL is registered, but 127.0.0.1:{#PgPort} accepts TCP. A server the ' +
      'registry does not know is not Setup''s to provision.' + RecoveryHint
  else if IsNonEmptyDirectory('{#PgDataDir}') then
    Result := 'No PostgreSQL is registered, but {#PgDataDir} exists and is not empty. Setup never ' +
      'overwrites existing data: remove the directory, for example after an interrupted ' +
      'install, then run Setup again.' + RecoveryHint
  else if WriterPassword = '' then
    Result := 'No scada_writer password: this run installs PostgreSQL 17 and creates the ' +
      'scada_writer role. Pass /WRITERPASSWORD=<password> or set SEMIBASE_WRITER_PASSWORD.';
end;

function EngineIsFresh: Boolean;
begin
  Result := not RegKeyExists(HKLM64, InstallationsKey + '\{#PgInstallation}');
end;

// The first matching state wins. An empty result fills Engine.
function EngineRefusal(const WriterPassword: String; var Engine: TEngine): String;
begin
  Engine.Fresh := EngineIsFresh;
  if not Engine.Fresh then
    Result := RegisteredEngineRefusal(Engine)
  else
  begin
    Result := OtherMajorRefusal;
    if Result = '' then
      Result := FreshInstallRefusal(WriterPassword);
  end;
end;

// EDB exits 0 without a superpassword and leaves a cluster nobody can log into, so the option
// file always carries one; EDB leaves the file in place, so Setup deletes it.
function InstallEngineRefusal(const SuperPassword: String): String;
var
  Options: TArrayOfString;
  OptionFile: String;
  Started: Boolean;
  ExitCode: Integer;
begin
  Result := '';
  ExtractTemporaryFile('{#PgInstaller}');
  OptionFile := ExpandConstant('{tmp}\postgresql-options.txt');
  SetArrayLength(Options, 5);
  Options[0] := 'superpassword=' + SuperPassword;
  Options[1] := 'serverport={#PgPort}';
  Options[2] := 'servicename={#PgService}';
  Options[3] := 'locale=C';
  Options[4] := 'datadir={#PgDataDir}';
  if not SaveStringsToFile(OptionFile, Options, False) then
  begin
    Result := 'Setup could not write the PostgreSQL option file into ' + ExpandConstant('{tmp}') +
      '.' + RecoveryHint;
    Exit;
  end;
  try
    Started := Exec(ExpandConstant('{tmp}\{#PgInstaller}'), '--mode unattended ' +
      '--unattendedmodeui none --optionfile "' + OptionFile + '" ' +
      '--disable-components pgAdmin,stackbuilder', '', SW_HIDE, ewWaitUntilTerminated, ExitCode);
  finally
    DeleteFile(OptionFile);
  end;
  if not Started then
    Result := 'Setup could not start the PostgreSQL installer: ' + SysErrorMessage(ExitCode) +
      '.' + RecoveryHint
  else if ExitCode <> 0 then
    Result := 'The PostgreSQL installer exited with code ' + IntToStr(ExitCode) + '; its log is ' +
      AddBackslash(GetEnv('TEMP')) + 'install-postgresql.log.' + RecoveryHint;
end;

// semibase site --restart-service refuses a Disabled or stopped service
function StartServiceRefusal(const ServiceName: String): String;
var
  ExitCode: Integer;
begin
  Result := '';
  ExitCode := ScExitCode('config "' + ServiceName + '" start= auto');
  if ExitCode <> 0 then
  begin
    Result := 'sc.exe config ' + ServiceName + ' start= auto exited with code ' +
      IntToStr(ExitCode) + '.' + RecoveryHint;
    Exit;
  end;
  ExitCode := ScExitCode('start "' + ServiceName + '"');
  if (ExitCode <> 0) and (ExitCode <> ErrorServiceAlreadyRunning) then
    Result := 'sc.exe start ' + ServiceName + ' exited with code ' + IntToStr(ExitCode) + '.' +
      RecoveryHint;
end;

function PortWaitRefusal(const ServiceName: String; Port: Cardinal): String;
var
  Started: Cardinal;
  Accepts: Boolean;
begin
  Result := '';
  Started := GetTickCount;
  repeat
    if not ProbePort(Port, Accepts) then
    begin
      Result := 'Setup could not probe 127.0.0.1:' + IntToStr(Port) + ' through powershell.exe.' +
        RecoveryHint;
      Exit;
    end;
    if Accepts then
      Exit;
    Sleep(1000);
  until GetTickCount - Started >= PortWaitMilliseconds;
  Result := 'Service ' + ServiceName + ' is started, but 127.0.0.1:' + IntToStr(Port) +
    ' accepted no TCP connection within 60 s.' + RecoveryHint;
end;

// semibase inherits the passwords from Setup's environment, so none is on a command line
function SiteRefusal(const Engine: TEngine; const SuperPassword, WriterPassword: String): String;
var
  SiteLog: String;
  ExitCode: Integer;
  Failed: Boolean;
begin
  Result := '';
  Failed := False;
  if not SetEnvironmentVariable('SEMIBASE_SUPER_PASSWORD', SuperPassword) then
    Failed := True;
  if not SetEnvironmentVariable('SEMIBASE_PLOT_PASSWORD', 'semiplot') then
    Failed := True;
  if WriterPassword <> '' then
    if not SetEnvironmentVariable('SEMIBASE_WRITER_PASSWORD', WriterPassword) then
      Failed := True;
  if Failed then
  begin
    Result := 'Setup could not set the SEMIBASE_* password variables for semibase site.';
    Exit;
  end;
  ExtractTemporaryFile('semibase.exe');
  ForceDirectories('{#LogRoot}');
  SiteLog := LogPath('site');
  if not Exec(ExpandConstant('{cmd}'), '/s /c ""' + ExpandConstant('{tmp}\semibase.exe') +
    '" site --port ' + IntToStr(Engine.Port) + ' --superuser "' + Engine.SuperUser +
    '" --expected-major 17 --restart-service "' + Engine.ServiceName + '" > "' + SiteLog +
    '" 2>&1"', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
    Result := 'Setup could not start cmd.exe to run semibase site: ' + SysErrorMessage(ExitCode) +
      '.'
  else if ExitCode <> 0 then
    Result := 'semibase site exited with code ' + IntToStr(ExitCode) + '; its log is ' + SiteLog +
      '.';
end;

var
  PasswordPage: TInputQueryWizardPage;

// The page holds the resolved passwords in every mode; under /SILENT nothing edits them
procedure InitializeWizard;
begin
  PasswordPage := CreateInputQueryPage(wpSelectDir, 'PostgreSQL passwords',
    'Setup provisions PostgreSQL 17 with these passwords.',
    'The superuser password is required on every run. The scada_writer password is required ' +
    'when Setup installs PostgreSQL 17; leave it empty on a server where scada_writer exists ' +
    'to keep its current password.');
  PasswordPage.Add('&Superuser password:', True);
  PasswordPage.Add('scada_&writer password:', True);
  PasswordPage.Values[0] := ResolvePassword('SUPERPASSWORD', 'SEMIBASE_SUPER_PASSWORD');
  PasswordPage.Values[1] := ResolvePassword('WRITERPASSWORD', 'SEMIBASE_WRITER_PASSWORD');
end;

// PrepareToInstall enforces every rule again; this only lets the user correct a field. A silent
// install simulates Next, and a False here would end it without exit code 7 or a setup log.
function NextButtonClick(CurPageID: Integer): Boolean;
var
  Refusal: String;
begin
  Result := True;
  if WizardSilent or (CurPageID <> PasswordPage.ID) then
    Exit;
  if PasswordPage.Values[0] = '' then
    Refusal := 'Enter the superuser password.'
  else
    Refusal := PasswordRefusal(PasswordPage.Values[0], PasswordPage.Values[1]);
  if (Refusal = '') and (PasswordPage.Values[1] = '') and EngineIsFresh and
    (OtherMajorRefusal = '') then
    Refusal := 'Enter the scada_writer password: no PostgreSQL 17 is registered, so Setup ' +
      'installs it and creates the scada_writer role.';
  if Refusal <> '' then
  begin
    MsgBox(Refusal, mbError, MB_OK);
    Result := False;
  end;
end;

// A refusal returned here comes before Inno copies a file and exits 7 under /VERYSILENT; one
// raised later comes after the uninstaller is registered and may still exit 0.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  SuperPassword, WriterPassword: String;
  Engine: TEngine;
begin
  SuperPassword := PasswordPage.Values[0];
  WriterPassword := PasswordPage.Values[1];
  Result := ParameterRefusal('SUPERPASSWORD', 'SEMIBASE_SUPER_PASSWORD');
  if Result = '' then
    Result := ParameterRefusal('WRITERPASSWORD', 'SEMIBASE_WRITER_PASSWORD');
  if Result = '' then
    Result := PasswordRefusal(SuperPassword, WriterPassword);
  if Result = '' then
    Result := EngineRefusal(WriterPassword, Engine);
  // The install's own registration, read like any other, fills Engine
  if (Result = '') and Engine.Fresh then
  begin
    Result := InstallEngineRefusal(SuperPassword);
    if Result = '' then
      Result := RegisteredEngineRefusal(Engine);
  end;
  if Result = '' then
    Result := StartServiceRefusal(Engine.ServiceName);
  if Result = '' then
    Result := PortWaitRefusal(Engine.ServiceName, Engine.Port);
  if Result = '' then
    Result := SiteRefusal(Engine, SuperPassword, WriterPassword);
  if Result <> '' then
    LogRefusal(Result);
end;
