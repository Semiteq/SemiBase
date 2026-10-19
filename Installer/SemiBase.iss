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

// A refusal returned here comes before Inno copies a file and exits 7 under /VERYSILENT; one
// raised later comes after the uninstaller is registered and may still exit 0.
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := ParameterRefusal('SUPERPASSWORD', 'SEMIBASE_SUPER_PASSWORD');
  if Result = '' then
    Result := ParameterRefusal('WRITERPASSWORD', 'SEMIBASE_WRITER_PASSWORD');
  if Result = '' then
    Result := PasswordRefusal(
      ResolvePassword('SUPERPASSWORD', 'SEMIBASE_SUPER_PASSWORD'),
      ResolvePassword('WRITERPASSWORD', 'SEMIBASE_WRITER_PASSWORD'));
end;
