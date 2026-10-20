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

; fetch-postgresql.sh saves the pinned build under this name and refuses a file that does not match the pin
#define PgInstaller  "postgresql-17-windows-x64.exe"

; Every run copies semibase.exe here, so a machine where Setup never succeeded still has the
; recovery tool. Outside {app}, so the uninstaller leaves it. Only administrators can create or
; change a folder in Program Files, so no user can plant a junction or a binary where Setup writes.
#define ToolRoot     "{commonpf}\SemiBase Setup"
#define LogRoot      ToolRoot + "\Logs"

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

; Program Files and the PostgreSQL service both need admin rights
PrivilegesRequired=admin

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#SemibaseExe}"; DestDir: "{app}"; DestName: "semibase.exe"; Flags: ignoreversion

; Stored as is, so extracting it in [Code] copies it instead of decompressing it
Source: "payload\{#PgInstaller}"; Flags: dontcopy nocompression

[Code]
function SetEnvironmentVariable(lpName, lpValue: String): BOOL;
  external 'SetEnvironmentVariableW@kernel32.dll stdcall';
function GetTickCount: Cardinal;
  external 'GetTickCount@kernel32.dll stdcall';

const
  InstallationsKey = 'SOFTWARE\PostgreSQL\Installations';
  ServicesKey = 'SOFTWARE\PostgreSQL\Services';
  RecoveryHint = ' See the recovery section of the SemiBase deployment guide, docs/deployment.md ' +
    'on GitHub.';
  PortWaitMilliseconds = 60000;
  ErrorServiceAlreadyRunning = 1056;

type
  TEngine = record
    ServiceName: String;
    Port: Cardinal;
    SuperUser: String;
  end;

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

// Setup's command-line parser drops every ", so a value typed with one arrives changed. The value
// must stand verbatim in the tail and end there: at the end, at a space, or at the " that closes a
// quote opened right before /NAME=. A trailing " would otherwise pass as a prefix match.
function ParameterReachesSetup(const Name, Value: String): Boolean;
var
  Prefix, Tail: String;
  PrefixStart, ValueEnd: Integer;
begin
  Prefix := '/' + Name + '=';
  Tail := GetCmdTail;
  PrefixStart := Pos(Uppercase(Prefix), Uppercase(Tail));
  ValueEnd := PrefixStart + Length(Prefix) + Length(Value);
  Result := (PrefixStart > 0) and (Copy(Tail, PrefixStart + Length(Prefix), Length(Value)) = Value);
  if Result and (ValueEnd <= Length(Tail)) and (Tail[ValueEnd] = '"') then
  begin
    Result := (PrefixStart > 1) and (Tail[PrefixStart - 1] = '"');
    ValueEnd := ValueEnd + 1;
  end;
  if Result then
    Result := (ValueEnd > Length(Tail)) or (Tail[ValueEnd] <= ' ');
end;

function ParameterRefusal(const Name, VariableName: String): String;
var
  Value: String;
begin
  Result := '';
  Value := CommandLineParameter(Name);
  if (Value <> '') and not ParameterReachesSetup(Name, Value) then
    Result := 'The /' + Name + ' value does not reach Setup as typed: Setup drops every " from ' +
      'its command line. Put the quotes around the whole parameter; a password that contains " ' +
      'reaches Setup only through ' + VariableName + '.';
end;

// An unquoted space splits a password parameter, and Setup would keep the part before it
function StrayArgumentRefusal: String;
var
  I: Integer;
begin
  Result := '';
  for I := 1 to ParamCount do
    if Copy(ParamStr(I), 1, 1) <> '/' then
    begin
      Result := 'Setup argument ' + IntToStr(I) + ' does not start with /. A space in an ' +
        'unquoted password parameter splits it there: put the quotes around the whole ' +
        'parameter, or pass the password through its SEMIBASE_* variable.';
      Exit;
    end;
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

// The scada_writer rule holds on every run: the SCADA logs in with that password from its own
// project settings, whose encoding of anything else nobody has measured
function PasswordRefusal(const SuperPassword, WriterPassword: String): String;
begin
  Result := '';
  if SuperPassword = '' then
    Result := 'No superuser password: pass /SUPERPASSWORD=<password> or set SEMIBASE_SUPER_PASSWORD.'
  else if not IsPrintableAscii(WriterPassword) then
    Result := 'The scada_writer password (/WRITERPASSWORD or SEMIBASE_WRITER_PASSWORD) must be ' +
      'printable ASCII.';
end;

// The PostgreSQL installer alone needs these rules, so they hold on the fresh-install row only
function FreshPasswordRefusal(const SuperPassword: String): String;
begin
  Result := '';
  if not IsPrintableAscii(SuperPassword) then
    Result := 'The superuser password must be printable ASCII: the PostgreSQL installer stores it ' +
      'in the ANSI code page, and semibase sends it in UTF-8.'
  else if (Pos('"', SuperPassword) > 0) or (SuperPassword[Length(SuperPassword)] = '\') then
    Result := 'The superuser password must not contain " or end in \: either one breaks the ' +
      'PostgreSQL installer''s command line.'
  else if (SuperPassword[1] = '-') or (SuperPassword[1] = ' ') or
    (SuperPassword[Length(SuperPassword)] = ' ') then
    Result := 'The superuser password must not start with -, or start or end with a space: the ' +
      'PostgreSQL installer passes it as a command-line argument and reads it from a key=value ' +
      'file, and neither is known to keep such a password.';
end;

function LogPath(const Prefix: String): String;
begin
  Result := ExpandConstant('{#LogRoot}\') + Prefix + '-' +
    GetDateTimeString('yyyymmdd-hhnnss', #0, #0) + '.log';
end;

// Under /VERYSILENT the returned refusal reaches no one, and /LOG would record the passwords
procedure LogRefusal(const Message: String);
var
  Lines: TArrayOfString;
begin
  SetArrayLength(Lines, 1);
  Lines[0] := Message;
  ForceDirectories(ExpandConstant('{#LogRoot}'));
  SaveStringsToUTF8FileWithoutBOM(LogPath('setup'), Lines, True);
end;

function KeepToolRefusal: String;
begin
  Result := '';
  ExtractTemporaryFile('semibase.exe');
  if not ForceDirectories(ExpandConstant('{#ToolRoot}')) or
    not CopyFile(ExpandConstant('{tmp}\semibase.exe'), ExpandConstant('{#ToolRoot}\semibase.exe'),
    False) then
    Result := 'Setup could not copy semibase.exe into ' + ExpandConstant('{#ToolRoot}') + '.' +
      RecoveryHint;
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

// Pascal Script has no sockets. The probe dials localhost, as semibase does, so ::1 and 127.0.0.1
// both count. Returns False when the probe itself failed, a constructor PowerShell refuses included.
function ProbePort(Port: Cardinal; var Accepts: Boolean): Boolean;
var
  ExitCode: Integer;
begin
  Result := Exec(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'),
    '-NoProfile -NonInteractive -Command "try { ([Net.Sockets.TcpClient]::new(''localhost'', ' +
    IntToStr(Port) + ')).Close(); exit 0 } catch { if ($_.Exception.InnerException -is ' +
    '[Net.Sockets.SocketException]) { exit 2 }; exit 3 }"',
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

// The cmd.exe line that runs semibase site expands % and ends its quoting at "
function IsCmdSafe(const Value: String): Boolean;
begin
  Result := (Pos('"', Value) = 0) and (Pos('%', Value) = 0);
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
  else if not (IsCmdSafe(SuperUser) and IsCmdSafe(ServiceName)) then
    Result := 'HKLM\' + Key + ' has a Super User or Service ID that contains " or %, which ' +
      'the command line that runs semibase site would change.' + RecoveryHint
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
function FreshInstallRefusal(const SuperPassword, WriterPassword: String): String;
var
  Accepts: Boolean;
begin
  Result := FreshPasswordRefusal(SuperPassword);
  if Result <> '' then
    Exit;
  if not ProbePort({#PgPort}, Accepts) then
    Result := 'Setup could not probe localhost:{#PgPort} through powershell.exe.' + RecoveryHint
  else if Accepts then
    Result := 'No PostgreSQL is registered, but localhost:{#PgPort} accepts TCP. A server the ' +
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

// The first matching state wins. An empty result fills Engine, except on the fresh row.
function EngineRefusal(Fresh: Boolean; const SuperPassword, WriterPassword: String;
  var Engine: TEngine): String;
begin
  if not Fresh then
    Result := RegisteredEngineRefusal(Engine)
  else
  begin
    Result := OtherMajorRefusal;
    if Result = '' then
      Result := FreshInstallRefusal(SuperPassword, WriterPassword);
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
      Result := 'Setup could not probe localhost:' + IntToStr(Port) + ' through powershell.exe.' +
        RecoveryHint;
      Exit;
    end;
    if Accepts then
      Exit;
    Sleep(1000);
  until GetTickCount - Started >= PortWaitMilliseconds;
  Result := 'Service ' + ServiceName + ' is started, but localhost:' + IntToStr(Port) +
    ' accepted no TCP connection within 60 s.' + RecoveryHint;
end;

// Read back rather than trusted: an empty value may delete the variable or store it empty, and
// semibase reads either as unset
function SetPasswordVariable(const Name, Value: String): Boolean;
begin
  SetEnvironmentVariable(Name, Value);
  Result := GetEnv(Name) = Value;
end;

// semibase inherits the passwords from Setup's environment, so none is on a command line. The
// writer variable is set even when empty, or a value inherited from the caller would reach semibase.
function SiteRefusal(const Engine: TEngine; const SuperPassword, WriterPassword: String): String;
var
  SiteLog: String;
  ExitCode: Integer;
begin
  Result := '';
  if not (SetPasswordVariable('SEMIBASE_SUPER_PASSWORD', SuperPassword) and
    SetPasswordVariable('SEMIBASE_WRITER_PASSWORD', WriterPassword) and
    SetPasswordVariable('SEMIBASE_PLOT_PASSWORD', 'semiplot')) then
  begin
    Result := 'Setup could not set the SEMIBASE_* password variables for semibase site.' +
      RecoveryHint;
    Exit;
  end;
  ForceDirectories(ExpandConstant('{#LogRoot}'));
  SiteLog := LogPath('site');
  if not Exec(ExpandConstant('{cmd}'), '/s /c ""' + ExpandConstant('{tmp}\semibase.exe') +
    '" site --port ' + IntToStr(Engine.Port) + ' --superuser "' + Engine.SuperUser +
    '" --expected-major 17 --restart-service "' + Engine.ServiceName + '" > "' + SiteLog +
    '" 2>&1"', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
    Result := 'Setup could not start cmd.exe to run semibase site: ' + SysErrorMessage(ExitCode) +
      '.' + RecoveryHint
  else if ExitCode <> 0 then
    Result := 'semibase site exited with code ' + IntToStr(ExitCode) + '; its log is ' + SiteLog +
      '.' + RecoveryHint;
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
  if (Refusal = '') and EngineIsFresh and (OtherMajorRefusal = '') then
  begin
    Refusal := FreshPasswordRefusal(PasswordPage.Values[0]);
    if (Refusal = '') and (PasswordPage.Values[1] = '') then
      Refusal := 'Enter the scada_writer password: no PostgreSQL 17 is registered, so Setup ' +
        'installs it and creates the scada_writer role.';
  end;
  if Refusal <> '' then
  begin
    MsgBox(Refusal, mbError, MB_OK);
    Result := False;
  end;
end;

// A refusal returned here comes before Inno copies a file and exits 7 under /VERYSILENT; one
// raised later comes after the uninstaller is registered and may still exit 0. An exception is
// turned into a refusal, so it reaches the setup log too.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  SuperPassword, WriterPassword: String;
  Fresh: Boolean;
  Engine: TEngine;
begin
  SuperPassword := PasswordPage.Values[0];
  WriterPassword := PasswordPage.Values[1];
  try
    Fresh := EngineIsFresh;
    Result := KeepToolRefusal;
    if Result = '' then
      Result := ParameterRefusal('SUPERPASSWORD', 'SEMIBASE_SUPER_PASSWORD');
    if Result = '' then
      Result := ParameterRefusal('WRITERPASSWORD', 'SEMIBASE_WRITER_PASSWORD');
    if Result = '' then
      Result := StrayArgumentRefusal;
    if Result = '' then
      Result := PasswordRefusal(SuperPassword, WriterPassword);
    if Result = '' then
      Result := EngineRefusal(Fresh, SuperPassword, WriterPassword, Engine);
    // The install's own registration, read like any other, fills Engine
    if (Result = '') and Fresh then
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
  except
    Result := 'Setup stopped on an error: ' + GetExceptionMessage + '.' + RecoveryHint;
  end;
  if Result <> '' then
    LogRefusal(Result);
end;
