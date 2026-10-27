; Run Installer/fetch-postgresql.sh first, through Git's bash.exe as CLAUDE.md's Build section
; shows; it puts the EDB installer into Installer\payload\.

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

#define LogRoot      "{app}\Logs"

; Equal to conventionalSuperPassword in cmd/semibase/main.go; the clean-install CI case logs in with it
#define ConventionalSuperPassword "postgres"

#define PgMajor        "17"
#define PgInstallation "postgresql-x64-" + PgMajor
#define PgService      "postgresql-x64-" + PgMajor
#define PgPort         "5432"
#define PgPrefix       "C:\Program Files\PostgreSQL\" + PgMajor
#define PgDataDir      PgPrefix + "\data"

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
DisableDirPage=yes
DisableProgramGroupPage=yes

SetupIconFile=SemiBase.ico
UninstallDisplayIcon={app}\SemiBase.ico

OutputDir=Output
OutputBaseFilename=SemiBase-Setup

Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes

WizardStyle=modern

PrivilegesRequired=admin

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

; The stock texts promise that every component goes; PostgreSQL, its data and the logs stay.
; DefaultDirName is fixed and DisableDirPage is set, so the logs folder is this one.
[Messages]
english.ConfirmUninstall=Remove %1? PostgreSQL {#PgMajor} with the archive and the logs stay.
russian.ConfirmUninstall=Удалить %1? PostgreSQL {#PgMajor} с архивом и журналы останутся.
english.UninstalledAll=%1 is removed. PostgreSQL {#PgMajor} with the archive and the logs in the folder C:\Program Files\SemiBase\Logs stay.
russian.UninstalledAll=%1 удалён. Остались PostgreSQL {#PgMajor} с архивом и журналы в папке C:\Program Files\SemiBase\Logs.

; The rules these texts follow: docs/architecture/installer-text.md. An entry without a language
; prefix applies to every language: it is a refusal's English detail, which reaches the setup log
; only.
[CustomMessages]
english.WriterPasswordNotAscii=The password of the database user scada_writer has characters that are not allowed. Use only Latin letters, digits, spaces and punctuation (printable ASCII).
russian.WriterPasswordNotAscii=Пароль пользователя БД scada_writer содержит недопустимые символы. Используйте только латинские буквы, цифры, пробелы и знаки препинания (печатные символы ASCII).
english.SuperPasswordRules=The password of the PostgreSQL administrator (database user postgres) is not allowed for a new PostgreSQL install. Use Latin letters, digits, spaces and punctuation (printable ASCII) with no ", no trailing \, no leading - and no leading or trailing space.
russian.SuperPasswordRules=Пароль администратора PostgreSQL (пользователь БД postgres) недопустим при новой установке PostgreSQL. Используйте латинские буквы, цифры, пробелы и знаки препинания (печатные символы ASCII) без кавычки ", без \ в конце, без - в начале и без пробела в начале или в конце.
english.CopyToolFailed=Setup could not copy the semibase.exe tool into the folder %1. Run Setup again.
russian.CopyToolFailed=Не удалось скопировать утилиту semibase.exe в папку %1. Запустите установщик снова.
IncompleteVersion=Version '%1'
IncompleteSuperUser=no Super User
IncompleteDataDirectory=no Data Directory
IncompletePgVersion=no file %1
IncompleteInstallationDetail=Registration %1: %2.
english.IncompleteInstallation=PostgreSQL %1 is installed incompletely. Remove PostgreSQL with %2\uninstall-postgresql.exe, wait until the removal ends, then run Setup again.
russian.IncompleteInstallation=PostgreSQL %1 установлен не полностью. Удалите PostgreSQL программой %2\uninstall-postgresql.exe, дождитесь окончания удаления и запустите установщик снова.
english.IncompleteInstallationNoFolder=PostgreSQL %1 is installed incompletely. Remove PostgreSQL %1 through Programs and Features (Control Panel), wait until the removal ends, then run Setup again.
russian.IncompleteInstallationNoFolder=PostgreSQL %1 установлен не полностью. Удалите PostgreSQL %1 через «Программы и компоненты» (панель управления), дождитесь окончания удаления и запустите установщик снова.
OrphanedRegistrationDetail=Registration %1, services %2: no Service ID or no Port.
english.OrphanedRegistration=PostgreSQL {#PgMajor} is installed, but Setup did not find its service name or its port. If the archive on this server is not needed, remove PostgreSQL with %1\uninstall-postgresql.exe, wait until the removal ends, then run Setup again.
russian.OrphanedRegistration=PostgreSQL {#PgMajor} установлен, но установщик не нашёл имя его службы или его порт. Если архив на этом сервере не нужен, удалите PostgreSQL программой %1\uninstall-postgresql.exe, дождитесь окончания удаления и запустите установщик снова.
OtherMajorDetail=Registered installations: %1.
english.OtherMajor=PostgreSQL of another version is installed, and Setup works with PostgreSQL %1 only. Move the data to PostgreSQL %1 or remove the other version, then run Setup again.
russian.OtherMajor=Установлен PostgreSQL другой версии, а установщик работает только с PostgreSQL %1. Перенесите данные на PostgreSQL %1 или удалите другую версию и запустите установщик снова.
english.ProbeFailed=Setup could not check through powershell.exe whether port %1 is free. Run Setup again.
russian.ProbeFailed=Не удалось проверить через powershell.exe, свободен ли порт %1. Запустите установщик снова.
NoRegistrationDetail=HKLM\SOFTWARE\PostgreSQL\Installations holds no registration.
english.PortBusy=Port %1 is busy, and no installed PostgreSQL is found. Stop the program that holds this port, then run Setup again.
russian.PortBusy=Порт %1 занят, а установленный PostgreSQL не найден. Остановите программу, которая занимает этот порт, и запустите установщик снова.
english.DataDirectoryNotEmpty=The folder %1 is not empty, and no installed PostgreSQL is found. If the data in this folder is not needed, delete it, then run Setup again.
russian.DataDirectoryNotEmpty=Папка %1 не пуста, а установленный PostgreSQL не найден. Если данные в этой папке не нужны, удалите её и запустите установщик снова.
english.OptionFileFailed=Setup could not write the option file of the PostgreSQL installer into the folder %1. Run Setup again.
russian.OptionFileFailed=Не удалось записать файл параметров установщика PostgreSQL в папку %1. Запустите установщик SemiBase снова.
english.EdbNotStarted=Setup could not start the PostgreSQL installer: %1. Run Setup again.
russian.EdbNotStarted=Не удалось запустить установщик PostgreSQL: %1. Запустите установщик SemiBase снова.
EdbFailedDetail=PostgreSQL installer exit code %1, its log %2.
english.EdbFailed=The PostgreSQL installer failed. Remove PostgreSQL with %1\uninstall-postgresql.exe, wait until the removal ends, then run Setup again.
russian.EdbFailed=Установщик PostgreSQL завершился с ошибкой. Удалите PostgreSQL программой %1\uninstall-postgresql.exe, дождитесь окончания удаления и запустите установщик SemiBase снова.
ServiceConfigFailedDetail=sc.exe config %1 start= auto exited with code %2.
english.ServiceConfigFailed=Setup could not set the startup type of the service %1 to Automatic: %2. If the service %1 does not exist, remove PostgreSQL with %3\uninstall-postgresql.exe, wait until the removal ends, then run Setup again.
russian.ServiceConfigFailed=Не удалось задать службе %1 тип запуска «Автоматически»: %2. Если службы %1 нет, удалите PostgreSQL программой %3\uninstall-postgresql.exe, дождитесь окончания удаления и запустите установщик снова.
ServiceStartFailedDetail=sc.exe start %1 exited with code %2.
english.ServiceStartFailed=The service %1 did not start: %2. Find the error in the server logs in the folder %3\log or in the Windows event log, fix it, then run Setup again.
russian.ServiceStartFailed=Служба %1 не запустилась: %2. Найдите ошибку в журналах сервера в папке %3\log или в журнале событий Windows, устраните её и запустите установщик снова.
english.SiteNotStarted=Setup could not start the semibase.exe tool: %1. Run Setup again.
russian.SiteNotStarted=Не удалось запустить утилиту semibase.exe: %1. Запустите установщик снова.
SitePasswordWrongDetail=semibase.exe site log: %1.
english.SitePasswordWrong=Wrong password of the PostgreSQL administrator (database user postgres). Enter the current password, then run Setup again.
russian.SitePasswordWrong=Неверный пароль администратора PostgreSQL (пользователь БД postgres). Укажите текущий пароль и запустите установщик снова.
SiteFailedDetail=semibase.exe exit code %1.
english.SiteFailed=The semibase.exe tool failed. Fix the error named in the last lines of the log %1, then run Setup again.
russian.SiteFailed=Утилита semibase.exe завершилась с ошибкой. Устраните ошибку, названную в последних строках журнала %1, и запустите установщик снова.
english.SetupError=Setup stopped on an error: %1. Run Setup again.
russian.SetupError=Установщик остановлен из-за ошибки: %1. Запустите установщик снова.
english.PasswordPageCaption=Database user passwords
russian.PasswordPageCaption=Пароли пользователей БД
english.PasswordPageDescriptionNew=Setup uses the passwords in the fields. For an empty field Setup takes the default password: the database user's name (postgres, scada_writer). PostgreSQL accepts connections from this computer only.
russian.PasswordPageDescriptionNew=Установщик использует пароли из полей. Для пустого поля установщик берёт пароль по умолчанию: имя пользователя БД (postgres, scada_writer). PostgreSQL принимает подключения только с этого компьютера.
english.PasswordPageDescriptionExisting=Setup connects to PostgreSQL %1 with the password below.
russian.PasswordPageDescriptionExisting=Установщик подключится к PostgreSQL %1 с паролем, указанным ниже.
english.SuperPasswordLabel=Password of the PostgreSQL administrator (database user postgres)
russian.SuperPasswordLabel=Пароль администратора PostgreSQL (пользователь БД postgres)
english.SuperPasswordHintNew=PostgreSQL %1 is installed with this password; a repeat run of Setup does not change it.
russian.SuperPasswordHintNew=PostgreSQL %1 будет установлен с этим паролем; повторный запуск установщика его не меняет.
english.SuperPasswordHintExisting=If the current password is different, replace the value in the field.
russian.SuperPasswordHintExisting=Если текущий пароль другой, замените значение в поле.
english.WriterPasswordLabel=Password of the database user scada_writer
russian.WriterPasswordLabel=Пароль пользователя БД scada_writer
english.WriterPasswordHint=The SCADA writes the archive with this password. Enter this password in the SCADA project (Project → Settings → Database); a repeat run of Setup does not change it.
russian.WriterPasswordHint=С этим паролем SCADA записывает архив. Укажите этот пароль в проекте SCADA (Проект → Настройки → База данных); повторный запуск установщика его не меняет.
english.ReadyPgInstall=PostgreSQL %1 will be installed
russian.ReadyPgInstall=PostgreSQL %1 будет установлен
english.ReadyPgRegistered=An installed PostgreSQL %1 is found, Setup will configure it
russian.ReadyPgRegistered=Найден установленный PostgreSQL %1, установщик настроит его
english.ReadyFolder=Folder: %1
russian.ReadyFolder=Папка: %1
english.ReadyData=Data folder: %1
russian.ReadyData=Папка данных: %1
english.ReadyServiceNew=Service: %1, startup type Automatic
russian.ReadyServiceNew=Служба: %1, тип запуска «Автоматически»
english.ReadyService=Service: %1, startup type becomes Automatic
russian.ReadyService=Служба: %1, тип запуска станет «Автоматически»
english.ReadyPortNew=Port: %1 (the PostgreSQL default); the SCADA and SemiPlot connect to it, from this computer only
russian.ReadyPortNew=Порт: %1 (порт PostgreSQL по умолчанию); к нему подключаются SCADA и SemiPlot, только с этого компьютера
english.ReadyPort=Port: %1; the SCADA and SemiPlot connect to it, from this computer only
russian.ReadyPort=Порт: %1; к нему подключаются SCADA и SemiPlot, только с этого компьютера
english.ReadyRestart=Setup restarts the PostgreSQL service when the server parameters shared_buffers or listen_addresses differ from the values Setup sets; on the first configuration of a server they differ. The SCADA connections drop for the restart: stop the SCADA before the install.
russian.ReadyRestart=Установщик перезапускает службу PostgreSQL, если параметры сервера shared_buffers или listen_addresses отличаются от значений, которые задаёт установщик; при первой настройке сервера они отличаются. Подключения SCADA на время перезапуска прервутся: остановите SCADA перед установкой.
english.ReadyTools=Logs: %1
russian.ReadyTools=Журналы: %1
english.ReadyCreateNew=The database scada_archive and these database users will be created:%nscada_writer for the SCADA to write the archive, password %1%nsemiplot for SemiPlot to read the archive, password semiplot
russian.ReadyCreateNew=Будут созданы база scada_archive и пользователи БД:%nscada_writer для записи архива SCADA, пароль %1%nsemiplot для чтения архива в SemiPlot, пароль semiplot
english.ReadyCreateExisting=The database scada_archive and the database users will be created if missing:%nscada_writer for the SCADA to write the archive, password %1; an existing user keeps its password%nsemiplot for SemiPlot to read the archive, the password is reset to semiplot
russian.ReadyCreateExisting=Будут созданы база scada_archive и пользователи БД, если их нет:%nscada_writer для записи архива SCADA, пароль %1; у существующего пароль не меняется%nsemiplot для чтения архива в SemiPlot, пароль сбрасывается на semiplot
english.ReadyRefusal=Setup cannot continue. %1%n%nClick Cancel.
russian.ReadyRefusal=Установка невозможна. %1%n%nНажмите «Отмена».
english.FinishedTextNew=PostgreSQL %1 is configured.%n%nEnter in the SCADA project (Project → Settings → Database):%nserver localhost%nport %2%ndatabase scada_archive%ndatabase user scada_writer%npassword %4%n%nSemiPlot connection:%nport %2%ndatabase user semiplot%npassword semiplot%n%nLogs: %3
russian.FinishedTextNew=PostgreSQL %1 настроен.%n%nУкажите в проекте SCADA (Проект → Настройки → База данных):%nсервер localhost%nпорт %2%nбаза scada_archive%nпользователь БД scada_writer%nпароль %4%n%nПодключение SemiPlot:%nпорт %2%nпользователь БД semiplot%nпароль semiplot%n%nЖурналы: %3
english.FinishedText=PostgreSQL %1 is configured.%n%nEnter in the SCADA project (Project → Settings → Database):%nserver localhost%nport %2%ndatabase scada_archive%ndatabase user scada_writer%npassword set when the user was created (scada_writer by default)%n%nSemiPlot connection:%nport %2%ndatabase user semiplot%npassword semiplot%n%nLogs: %3
russian.FinishedText=PostgreSQL %1 настроен.%n%nУкажите в проекте SCADA (Проект → Настройки → База данных):%nсервер localhost%nпорт %2%nбаза scada_archive%nпользователь БД scada_writer%nпароль, заданный при создании пользователя (по умолчанию scada_writer)%n%nПодключение SemiPlot:%nпорт %2%nпользователь БД semiplot%nпароль semiplot%n%nЖурналы: %3

[Files]
Source: "{#SemibaseExe}"; DestDir: "{app}"; DestName: "semibase.exe"; Flags: ignoreversion
Source: "SemiBase.ico"; DestDir: "{app}"; Flags: ignoreversion

Source: "payload\{#PgInstaller}"; Flags: dontcopy nocompression

[Code]
function SetEnvironmentVariable(lpName, lpValue: String): BOOL;
  external 'SetEnvironmentVariableW@kernel32.dll stdcall';

const
  InstallationsKey = 'SOFTWARE\PostgreSQL\Installations';
  ServicesKey = 'SOFTWARE\PostgreSQL\Services';
  ErrorServiceAlreadyRunning = 1056;

type
  TEngine = record
    ServiceName: String;
    Port: Cardinal;
    SuperUser: String;
    BaseDirectory: String;
    DataDirectory: String;
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

// Holds on every run: docs/architecture/installer.md#passwords
function WriterPasswordRefusal(const WriterPassword: String): String;
begin
  Result := '';
  if not IsPrintableAscii(WriterPassword) then
    Result := CustomMessage('WriterPasswordNotAscii');
end;

function SuperPasswordOrConvention(const SuperPassword: String): String;
begin
  Result := SuperPassword;
  if Result = '' then
    Result := '{#ConventionalSuperPassword}';
end;

function WriterPasswordOrConvention(const WriterPassword: String): String;
begin
  Result := WriterPassword;
  if Result = '' then
    Result := 'scada_writer';
end;

// The PostgreSQL installer alone needs these rules, so they hold on the fresh-install row only.
// SuperPassword is never empty: the caller resolves the convention first.
function FreshPasswordRefusal(const SuperPassword: String): String;
begin
  Result := '';
  if not IsPrintableAscii(SuperPassword) or (Pos('"', SuperPassword) > 0) or
    (SuperPassword[Length(SuperPassword)] = '\') or (SuperPassword[1] = '-') or
    (SuperPassword[1] = ' ') or (SuperPassword[Length(SuperPassword)] = ' ') then
    Result := CustomMessage('SuperPasswordRules');
end;

// Creates the log folder, which KeepToolRefusal may not have reached
function LogPath(const Prefix: String): String;
begin
  ForceDirectories(ExpandConstant('{#LogRoot}'));
  Result := ExpandConstant('{#LogRoot}\') + Prefix + '-' +
    GetDateTimeString('yyyymmdd-hhnnss', #0, #0) + '.log';
end;

procedure AppendLine(const Path, Line: String);
var
  Lines: TArrayOfString;
begin
  SetArrayLength(Lines, 1);
  Lines[0] := Line;
  SaveStringsToUTF8FileWithoutBOM(Path, Lines, True);
end;

var
  RefusalDetail: String;

// The detail lands in the setup log on the line before the refusal, never on screen
function Refuse(const Text, Detail: String): String;
begin
  RefusalDetail := Detail;
  Result := Text;
end;

// Under /VERYSILENT the returned refusal reaches no one. One path for both lines, since each
// LogPath call stamps the current second.
procedure LogRefusal(const Message: String);
var
  Path: String;
begin
  Path := LogPath('setup');
  if RefusalDetail <> '' then
    AppendLine(Path, RefusalDetail);
  AppendLine(Path, Message);
end;

// EDB writes Base Directory in every registration measured, a failed install's included; only a
// hand-made key lacks it, and the uninstaller then sits in EDB's default prefix
function UninstallerFolder(const BaseDirectory: String): String;
begin
  Result := BaseDirectory;
  if Result = '' then
    Result := '{#PgPrefix}';
end;

// The [Files] entry later replaces this copy and registers it for the uninstaller
function KeepToolRefusal: String;
begin
  Result := '';
  ExtractTemporaryFile('semibase.exe');
  if not ForceDirectories(ExpandConstant('{app}')) or
    not CopyFile(ExpandConstant('{tmp}\semibase.exe'), ExpandConstant('{app}\semibase.exe'),
    False) then
    Result := FmtMessage(CustomMessage('CopyToolFailed'), [ExpandConstant('{app}')]);
end;

// sc.exe exits with a Win32 error code, and an Exec that fails leaves one in the same place, so
// SysErrorMessage reads either
function ScExitCode(const Arguments: String): Integer;
var
  ExitCode: Integer;
begin
  if not Exec(ExpandConstant('{sys}\sc.exe'), Arguments, '', SW_HIDE, ewWaitUntilTerminated,
    ExitCode) and (ExitCode = 0) then
    ExitCode := -1;
  Result := ExitCode;
end;

// The probe dials localhost, as semibase does, so ::1 and 127.0.0.1 both count. Returns False
// when the probe itself failed, a constructor PowerShell refuses included.
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

// The {#PgInstallation} key exists: it is used only when it is a complete install. An empty
// Service ID would reach site as --restart-service ""; a Service ID naming no service is refused
// by sc.exe config, and a wrong Port by site.
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
  RegQueryStringValue(HKLM64, Key, 'Base Directory', BaseDirectory);

  if Copy(Version, 1, Length('{#PgMajor}.')) <> '{#PgMajor}.' then
    Missing := FmtMessage(CustomMessage('IncompleteVersion'), [Version])
  else if SuperUser = '' then
    Missing := CustomMessage('IncompleteSuperUser')
  else if DataDirectory = '' then
    Missing := CustomMessage('IncompleteDataDirectory')
  else if not FileExists(AddBackslash(DataDirectory) + 'PG_VERSION') then
    Missing := FmtMessage(CustomMessage('IncompletePgVersion'), [
      AddBackslash(DataDirectory) + 'PG_VERSION']);
  if Missing <> '' then
  begin
    if BaseDirectory <> '' then
      Result := Refuse(FmtMessage(CustomMessage('IncompleteInstallation'), ['{#PgMajor}',
        BaseDirectory]), FmtMessage(CustomMessage('IncompleteInstallationDetail'), [
        'HKLM\' + Key, Missing]))
    else
      Result := Refuse(FmtMessage(CustomMessage('IncompleteInstallationNoFolder'), [
        '{#PgMajor}']), FmtMessage(CustomMessage('IncompleteInstallationDetail'), [
        'HKLM\' + Key, Missing]));
    Exit;
  end;

  if (ServiceName = '') or
    not RegQueryDWordValue(HKLM64, ServicesKey + '\' + ServiceName, 'Port', Port) then
  begin
    Result := Refuse(FmtMessage(CustomMessage('OrphanedRegistration'), [
      UninstallerFolder(BaseDirectory)]), FmtMessage(CustomMessage('OrphanedRegistrationDetail'), [
      'HKLM\' + Key, 'HKLM\' + ServicesKey]));
    Exit;
  end;

  Engine.ServiceName := ServiceName;
  Engine.Port := Port;
  Engine.SuperUser := SuperUser;
  Engine.BaseDirectory := BaseDirectory;
  Engine.DataDirectory := DataDirectory;
end;

// Every key name under Installations, comma-separated; empty when there is none
function RegisteredInstallations: String;
var
  Names: TArrayOfString;
  I: Integer;
begin
  Result := '';
  if not RegGetSubkeyNames(HKLM64, InstallationsKey, Names) then
    Exit;
  for I := 0 to GetArrayLength(Names) - 1 do
  begin
    if Result <> '' then
      Result := Result + ', ';
    Result := Result + Names[I];
  end;
end;

// The {#PgInstallation} key does not exist, and another major may be registered
function OtherMajorRefusal: String;
var
  Registered: String;
begin
  Result := '';
  Registered := RegisteredInstallations;
  if Registered <> '' then
    Result := Refuse(FmtMessage(CustomMessage('OtherMajor'), ['{#PgMajor}']),
      FmtMessage(CustomMessage('OtherMajorDetail'), [Registered]));
end;

// Nothing is registered: a fresh install must not meet a server or data it does not know
function FreshMachineRefusal: String;
var
  Accepts: Boolean;
begin
  Result := '';
  if not ProbePort({#PgPort}, Accepts) then
    Result := FmtMessage(CustomMessage('ProbeFailed'), ['{#PgPort}'])
  else if Accepts then
    Result := Refuse(FmtMessage(CustomMessage('PortBusy'), ['{#PgPort}']),
      CustomMessage('NoRegistrationDetail'))
  else if IsNonEmptyDirectory('{#PgDataDir}') then
    Result := Refuse(FmtMessage(CustomMessage('DataDirectoryNotEmpty'), ['{#PgDataDir}']),
      CustomMessage('NoRegistrationDetail'));
end;

// The password page passes ProbeMachine False: a probe of a closed port takes about 7 s, and the
// page checks only what the user can correct in a field.
function FreshInstallRefusal(const SuperPassword: String; ProbeMachine: Boolean): String;
begin
  Result := FreshPasswordRefusal(SuperPassword);
  if (Result = '') and ProbeMachine then
    Result := FreshMachineRefusal;
end;

function EngineIsFresh: Boolean;
begin
  Result := not RegKeyExists(HKLM64, InstallationsKey + '\{#PgInstallation}');
end;

// The refusals the registry alone decides, with no probe: the wizard pages show them, and
// PrepareToInstall finds them again through EngineRefusal
function RegistryRefusal(var Engine: TEngine): String;
begin
  if EngineIsFresh then
    Result := OtherMajorRefusal
  else
    Result := RegisteredEngineRefusal(Engine);
end;

// The first matching state wins. An empty result fills Engine, except on the fresh row.
function EngineRefusal(Fresh: Boolean; const SuperPassword: String; var Engine: TEngine): String;
begin
  if not Fresh then
    Result := RegisteredEngineRefusal(Engine)
  else
  begin
    Result := OtherMajorRefusal;
    if Result = '' then
      Result := FreshInstallRefusal(SuperPassword, True);
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
    Result := FmtMessage(CustomMessage('OptionFileFailed'), [ExpandConstant('{tmp}')]);
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
    Result := FmtMessage(CustomMessage('EdbNotStarted'), [SysErrorMessage(ExitCode)])
  else if ExitCode <> 0 then
    Result := Refuse(FmtMessage(CustomMessage('EdbFailed'), ['{#PgPrefix}']),
      FmtMessage(CustomMessage('EdbFailedDetail'), [IntToStr(ExitCode),
      AddBackslash(GetEnv('TEMP')) + 'install-postgresql.log']));
end;

// semibase site --restart-service refuses a Disabled or stopped service
function StartServiceRefusal(const Engine: TEngine): String;
var
  ExitCode: Integer;
begin
  Result := '';
  ExitCode := ScExitCode('config "' + Engine.ServiceName + '" start= auto');
  if ExitCode <> 0 then
  begin
    Result := Refuse(FmtMessage(CustomMessage('ServiceConfigFailed'), [Engine.ServiceName,
      SysErrorMessage(ExitCode), UninstallerFolder(Engine.BaseDirectory)]),
      FmtMessage(CustomMessage('ServiceConfigFailedDetail'), [Engine.ServiceName,
      IntToStr(ExitCode)]));
    Exit;
  end;
  ExitCode := ScExitCode('start "' + Engine.ServiceName + '"');
  if (ExitCode <> 0) and (ExitCode <> ErrorServiceAlreadyRunning) then
    Result := Refuse(FmtMessage(CustomMessage('ServiceStartFailed'), [Engine.ServiceName,
      SysErrorMessage(ExitCode), Engine.DataDirectory]),
      FmtMessage(CustomMessage('ServiceStartFailedDetail'), [Engine.ServiceName,
      IntToStr(ExitCode)]));
end;

var
  SiteLog: String;
  SitePasswordRejected: Boolean;

// Each line lands as it arrives, so a Setup killed during a long service stop still leaves a log.
// semibase prints the SQLSTATE of a refused password in English whatever the server's language.
procedure AppendSiteLine(const S: String; const Error, FirstLine: Boolean);
begin
  AppendLine(SiteLog, S);
  if Pos('SQLSTATE 28P01', S) > 0 then
    SitePasswordRejected := True;
end;

// semibase inherits the passwords from Setup's environment, so none is on a command line. Both
// variables are set even when empty, or a value inherited from the caller would reach semibase;
// semibase reads an empty one as unset and applies its convention.
function SiteRefusal(const Engine: TEngine; const SuperPassword, WriterPassword: String): String;
var
  ExitCode: Integer;
begin
  Result := '';
  SetEnvironmentVariable('SEMIBASE_SUPER_PASSWORD', SuperPassword);
  SetEnvironmentVariable('SEMIBASE_WRITER_PASSWORD', WriterPassword);
  SiteLog := LogPath('site');
  SitePasswordRejected := False;
  if not ExecAndLogOutput(ExpandConstant('{tmp}\semibase.exe'), 'site --port ' +
    IntToStr(Engine.Port) + ' --superuser "' + Engine.SuperUser + '" --expected-major ' +
    '{#PgMajor} --restart-service "' + Engine.ServiceName + '"', '', SW_SHOWNORMAL,
    ewWaitUntilTerminated, ExitCode, @AppendSiteLine) then
    Result := FmtMessage(CustomMessage('SiteNotStarted'), [SysErrorMessage(ExitCode)])
  else if (ExitCode <> 0) and SitePasswordRejected then
    Result := Refuse(CustomMessage('SitePasswordWrong'),
      FmtMessage(CustomMessage('SitePasswordWrongDetail'), [SiteLog]))
  else if ExitCode <> 0 then
    Result := Refuse(FmtMessage(CustomMessage('SiteFailed'), [SiteLog]),
      FmtMessage(CustomMessage('SiteFailedDetail'), [IntToStr(ExitCode)]));
end;

var
  PasswordPage: TWizardPage;
  SuperEdit, WriterEdit: TNewEdit;
  Provisioned: TEngine;
  // EngineIsFresh is false once the install registered 17, so PrepareToInstall keeps its answer
  ProvisionedFresh: Boolean;
  Refused: Boolean;
  ReadyRefusalLogged: Boolean;

// A bold prompt, its own wrapped explanation, then a plain edit: the passwords are conventions,
// so they are shown, not masked
function AddPasswordField(Page: TWizardPage; var Top: Integer;
  const Prompt, Hint, Value: String): TNewEdit;
var
  PromptLabel, HintLabel: TNewStaticText;
begin
  PromptLabel := TNewStaticText.Create(Page);
  PromptLabel.Parent := Page.Surface;
  PromptLabel.Top := Top;
  PromptLabel.Font.Style := [fsBold];
  PromptLabel.Caption := Prompt;
  HintLabel := TNewStaticText.Create(Page);
  HintLabel.Parent := Page.Surface;
  HintLabel.Top := PromptLabel.Top + PromptLabel.Height + ScaleY(4);
  HintLabel.Width := Page.SurfaceWidth;
  HintLabel.WordWrap := True;
  HintLabel.Caption := Hint;
  Result := TNewEdit.Create(Page);
  Result.Parent := Page.Surface;
  Result.Top := HintLabel.Top + HintLabel.Height + ScaleY(6);
  Result.Width := Page.SurfaceWidth;
  Result.Text := Value;
  PromptLabel.FocusControl := Result;
  Top := Result.Top + Result.Height + ScaleY(18);
end;

// The page holds the passwords in every mode; under /SILENT nothing edits them
procedure InitializeWizard;
var
  Top: Integer;
  Description, SuperHint: String;
begin
  WizardForm.ReadyMemo.WordWrap := True;
  WizardForm.ReadyMemo.ScrollBars := ssVertical;
  if EngineIsFresh then
  begin
    Description := CustomMessage('PasswordPageDescriptionNew');
    SuperHint := 'SuperPasswordHintNew';
  end
  else
  begin
    Description := FmtMessage(CustomMessage('PasswordPageDescriptionExisting'), ['{#PgMajor}']);
    SuperHint := 'SuperPasswordHintExisting';
  end;
  PasswordPage := CreateCustomPage(wpSelectDir, CustomMessage('PasswordPageCaption'), Description);
  Top := 0;
  SuperEdit := AddPasswordField(PasswordPage, Top, CustomMessage('SuperPasswordLabel'),
    FmtMessage(CustomMessage(SuperHint), ['{#PgMajor}']),
    SuperPasswordOrConvention(GetEnv('SEMIBASE_SUPER_PASSWORD')));
  // The writer's password is set only when the user is created, which only a fresh install is
  // sure to do; on a registered 17 semibase creates a missing user with the convention.
  if EngineIsFresh then
    WriterEdit := AddPasswordField(PasswordPage, Top, CustomMessage('WriterPasswordLabel'),
      CustomMessage('WriterPasswordHint'),
      WriterPasswordOrConvention(GetEnv('SEMIBASE_WRITER_PASSWORD')));
end;

// The field when the page has one, else the caller's variable; semibase applies it on creation only
function WriterPasswordValue: String;
begin
  if WriterEdit <> nil then
    Result := WriterEdit.Text
  else
    Result := GetEnv('SEMIBASE_WRITER_PASSWORD');
end;

// A machine Setup refuses whatever the passwords has nothing to ask for; the Ready page says why
function ShouldSkipPage(PageID: Integer): Boolean;
var
  Engine: TEngine;
begin
  Result := (PageID = PasswordPage.ID) and (RegistryRefusal(Engine) <> '');
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
  Refusal := WriterPasswordRefusal(WriterPasswordValue);
  if (Refusal = '') and EngineIsFresh then
    Refusal := FreshInstallRefusal(SuperPasswordOrConvention(SuperEdit.Text), False);
  if Refusal <> '' then
  begin
    MsgBox(Refusal, mbError, MB_OK);
    Result := False;
  end;
end;

// A text whose %n-separated items follow its heading, each on its own deeper line
function ReadyList(const Text, Space, NewLine: String): String;
begin
  Result := Text;
  StringChangeEx(Result, #13#10, NewLine + Space + Space, True);
end;

// The registry reads alone; PrepareToInstall decides. Inno runs this on every visit to the page
// and under /SILENT too, where PrepareToInstall logs the refusal anyway; a user who clicks Cancel
// as the page says never reaches PrepareToInstall, so the page logs its refusal once.
function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
var
  Engine: TEngine;
  Refusal, Tools, WriterPassword: String;
begin
  Refusal := RegistryRefusal(Engine);
  if Refusal <> '' then
  begin
    if not WizardSilent and not ReadyRefusalLogged then
    begin
      LogRefusal(Refusal);
      ReadyRefusalLogged := True;
    end;
    Result := FmtMessage(CustomMessage('ReadyRefusal'), [Refusal]);
    Exit;
  end;
  Tools := '{#AppName}' + NewLine +
    Space + FmtMessage(CustomMessage('ReadyTools'), [ExpandConstant('{#LogRoot}')]) + NewLine;
  WriterPassword := WriterPasswordOrConvention(WriterPasswordValue);
  if EngineIsFresh then
  begin
    Result := FmtMessage(CustomMessage('ReadyPgInstall'), ['{#PgMajor}']) + NewLine +
      Space + FmtMessage(CustomMessage('ReadyFolder'), ['{#PgPrefix}']) + NewLine +
      Space + FmtMessage(CustomMessage('ReadyData'), ['{#PgDataDir}']) + NewLine +
      Space + FmtMessage(CustomMessage('ReadyServiceNew'), ['{#PgService}']) + NewLine +
      Space + FmtMessage(CustomMessage('ReadyPortNew'), ['{#PgPort}']) + NewLine + NewLine +
      Tools + Space + ReadyList(FmtMessage(CustomMessage('ReadyCreateNew'), [WriterPassword]),
      Space, NewLine);
    Exit;
  end;
  Result := FmtMessage(CustomMessage('ReadyPgRegistered'), ['{#PgMajor}']) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyFolder'), [Engine.BaseDirectory]) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyService'), [Engine.ServiceName]) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyPort'), [IntToStr(Engine.Port)]) + NewLine + NewLine +
    CustomMessage('ReadyRestart') + NewLine + NewLine +
    Tools + Space + ReadyList(FmtMessage(CustomMessage('ReadyCreateExisting'), [WriterPassword]),
    Space, NewLine);
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
  SuperPassword := SuperEdit.Text;
  WriterPassword := WriterPasswordValue;
  try
    Fresh := EngineIsFresh;
    RefusalDetail := '';
    Result := KeepToolRefusal;
    if Result = '' then
      Result := WriterPasswordRefusal(WriterPassword);
    if Result = '' then
      Result := EngineRefusal(Fresh, SuperPasswordOrConvention(SuperPassword), Engine);
    // The install's own registration, read like any other, fills Engine
    if (Result = '') and Fresh then
    begin
      Result := InstallEngineRefusal(SuperPasswordOrConvention(SuperPassword));
      if Result = '' then
        Result := RegisteredEngineRefusal(Engine);
    end;
    if Result = '' then
      Result := StartServiceRefusal(Engine);
    if Result = '' then
      Result := SiteRefusal(Engine, SuperPassword, WriterPassword);
  except
    RefusalDetail := '';
    Result := FmtMessage(CustomMessage('SetupError'), [GetExceptionMessage]);
  end;
  Refused := Result <> '';
  if Refused then
    LogRefusal(Result)
  else
  begin
    Provisioned := Engine;
    ProvisionedFresh := Fresh;
  end;
end;

// PreparingLabel is static text; the memo lets the reader select and copy a path in the refusal
procedure ShowRefusalAsText;
var
  Memo: TNewMemo;
begin
  Memo := WizardForm.PreparingMemo;
  Memo.Top := WizardForm.PreparingLabel.Top;
  Memo.Height := WizardForm.PreparingPage.Height - Memo.Top;
  Memo.WordWrap := True;
  Memo.ScrollBars := ssVertical;
  Memo.Text := WizardForm.PreparingLabel.Caption;
  WizardForm.PreparingLabel.Visible := False;
  Memo.Visible := True;
end;

// Inno enters wpPreparing once before PrepareToInstall, with the label hidden, and once after a
// refusal, with the label showing it; the applications-in-use list also shows the label and the
// memo, but with no refusal. Under /VERYSILENT no page is shown, and the site log carries the
// Finished values.
procedure CurPageChanged(CurPageID: Integer);
begin
  if (CurPageID = wpPreparing) and Refused and WizardForm.PreparingLabel.Visible then
    ShowRefusalAsText
  else if CurPageID = wpFinished then
  begin
    if ProvisionedFresh then
      WizardForm.FinishedLabel.Caption := FmtMessage(CustomMessage('FinishedTextNew'), [
        '{#PgMajor}', IntToStr(Provisioned.Port), ExpandConstant('{#LogRoot}'),
        WriterPasswordOrConvention(WriterPasswordValue)])
    else
      WizardForm.FinishedLabel.Caption := FmtMessage(CustomMessage('FinishedText'), [
        '{#PgMajor}', IntToStr(Provisioned.Port), ExpandConstant('{#LogRoot}')]);
    WizardForm.AdjustLabelHeight(WizardForm.FinishedLabel);
  end;
end;
