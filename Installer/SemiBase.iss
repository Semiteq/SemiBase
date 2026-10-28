; Run Installer/fetch-postgresql.sh first, through Git's bash.exe as AGENTS.md's Build section
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
EngineFailedDetail=semibase.exe engine exit code %1: %2
english.EngineFailed=The semibase.exe tool did not determine the state of PostgreSQL. Run Setup again.
russian.EngineFailed=Утилита semibase.exe не определила состояние PostgreSQL. Запустите установщик снова.
english.AmbiguousEngine=Several services that run PostgreSQL {#PgMajor} or PostgreSQL of an unknown version are found: %1. Open Command Prompt with Run as administrator, run sc.exe stop <service name> and sc.exe delete <service name> for each service the SCADA does not use, then run Setup again.
russian.AmbiguousEngine=Найдено несколько служб PostgreSQL {#PgMajor} или PostgreSQL неизвестной версии: %1. Откройте программу «Командная строка» командой «Запуск от имени администратора», выполните sc.exe stop <имя службы> и sc.exe delete <имя службы> для каждой службы, которую не использует SCADA, и запустите установщик снова.
english.IncompleteInstallation=The PostgreSQL service %1 is installed incompletely. Remove PostgreSQL with %2\uninstall-postgresql.exe, wait until the removal ends, then run Setup again.
russian.IncompleteInstallation=Служба PostgreSQL %1 установлена не полностью. Удалите PostgreSQL программой %2\uninstall-postgresql.exe, дождитесь окончания удаления и запустите установщик снова.
english.IncompleteInstallationNoUninstaller=The PostgreSQL service %1 is installed incompletely. Open Command Prompt with Run as administrator, run sc.exe stop %1 and sc.exe delete %1, then run Setup again.
russian.IncompleteInstallationNoUninstaller=Служба PostgreSQL %1 установлена не полностью. Откройте программу «Командная строка» командой «Запуск от имени администратора», выполните sc.exe stop %1 и sc.exe delete %1 и запустите установщик снова.
english.UnreadableService=Setup could not read the settings of the PostgreSQL service %1: %2. Fix the error, then run Setup again.
russian.UnreadableService=Не удалось прочитать настройки службы PostgreSQL %1: %2. Устраните ошибку и запустите установщик снова.
english.PortMismatch=PostgreSQL {#PgMajor} runs on port %1, and the file %2 sets port %3. Restart the service %4 so that PostgreSQL moves to port %3, or set port %1 in the file, then run Setup again.
russian.PortMismatch=PostgreSQL {#PgMajor} работает на порту %1, а в файле %2 указан порт %3. Перезапустите службу %4, чтобы PostgreSQL перешёл на порт %3, или укажите в файле порт %1, затем запустите установщик снова.
english.PortMismatchNoFile=PostgreSQL {#PgMajor} runs on port %1, and the service %2 starts it on port %3. Restart the service %2, then run Setup again.
russian.PortMismatchNoFile=PostgreSQL {#PgMajor} работает на порту %1, а служба %2 запускает его на порту %3. Перезапустите службу %2 и запустите установщик снова.
english.OtherMajor=PostgreSQL of another version is installed, and Setup works with PostgreSQL %1 only. Move the data to PostgreSQL %1 or remove the other version, then run Setup again.
russian.OtherMajor=Установлен PostgreSQL другой версии, а установщик работает только с PostgreSQL %1. Перенесите данные на PostgreSQL %1 или удалите другую версию и запустите установщик снова.
UninstallerRunningDetail=PostgreSQL uninstaller process running: %1.
UninstallerProbeFailedDetail=Setup could not check through powershell.exe whether a PostgreSQL uninstaller runs (%1), and goes on as if none runs.
english.UninstallerRunning=The removal of PostgreSQL has not ended yet. Wait until the PostgreSQL uninstaller closes, then run Setup again.
russian.UninstallerRunning=Удаление PostgreSQL ещё не завершено. Дождитесь, пока закроется программа удаления PostgreSQL, и запустите установщик снова.
english.ProbeFailed=Setup could not check through powershell.exe whether port %1 is free. Run Setup again.
russian.ProbeFailed=Не удалось проверить через powershell.exe, свободен ли порт %1. Запустите установщик снова.
PortOwnerDetail=port %1 held by %2
PortNoOwnerDetail=port %1 accepts connections, and Get-NetTCPConnection names no listening process
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
english.ServiceConfigFailed=Setup could not set the startup type of the service %1 to Automatic: %2. Fix the error, then run Setup again.
russian.ServiceConfigFailed=Не удалось задать службе %1 тип запуска «Автоматически»: %2. Устраните ошибку и запустите установщик снова.
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
english.ReadyPgFound=An installed PostgreSQL %1 is found, Setup will configure it
russian.ReadyPgFound=Найден установленный PostgreSQL %1, установщик настроит его
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
  ErrorServiceAlreadyRunning = 1056;
  // The state= values semibase engine prints (State in internal/engine/engine.go)
  StateAmbiguous = 'ambiguous';
  StateIncomplete = 'incomplete';
  StateUnreadable = 'unreadable';
  StatePortMismatch = 'port-mismatch';
  StateFound = 'found';
  StateOtherMajor = 'other-major';
  StateFresh = 'fresh';

type
  // The lines semibase.exe engine prints. State is empty when the command did not report one;
  // ExecError is then set when the command did not start.
  TEngine = record
    State: String;
    Services: String;
    ServiceName: String;
    BaseDirectory: String;
    DataDirectory: String;
    Port: String;
    PortFile: String;
    Listening: String;
    Uninstaller: Boolean;
    Unreadable: String;
    Detail: String;
    ExecError: String;
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

function PowerShellExe: String;
begin
  Result := ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe');
end;

function JoinLines(const Lines: TArrayOfString): String;
var
  I: Integer;
begin
  Result := '';
  for I := 0 to GetArrayLength(Lines) - 1 do
  begin
    if Result <> '' then
      Result := Result + ', ';
    Result := Result + Lines[I];
  end;
end;

// A detail= value may hold '=', so each line splits at its first one.
procedure DetectEngine(var Engine: TEngine);
var
  Empty: TEngine;
  Started: Boolean;
  ExitCode, I, Separator: Integer;
  Output: TExecOutput;
  Key, Value: String;
begin
  Engine := Empty;
  try
    ExtractTemporaryFile('semibase.exe');
    Started := ExecAndCaptureOutput(ExpandConstant('{tmp}\semibase.exe'),
      'engine --expected-major {#PgMajor}', '', SW_HIDE, ewWaitUntilTerminated, ExitCode, Output);
    if not Started then
      Engine.ExecError := SysErrorMessage(ExitCode);
  except
    Engine.ExecError := GetExceptionMessage;
  end;
  if Engine.ExecError <> '' then
    Exit;
  if ExitCode <> 0 then
  begin
    Engine.Detail := FmtMessage(CustomMessage('EngineFailedDetail'), [IntToStr(ExitCode),
      JoinLines(Output.StdErr)]);
    Exit;
  end;
  for I := 0 to GetArrayLength(Output.StdOut) - 1 do
  begin
    Separator := Pos('=', Output.StdOut[I]);
    Key := Copy(Output.StdOut[I], 1, Separator - 1);
    Value := Copy(Output.StdOut[I], Separator + 1, Length(Output.StdOut[I]));
    if Key = 'state' then
      Engine.State := Value
    else if Key = 'services' then
      Engine.Services := Value
    else if Key = 'service' then
      Engine.ServiceName := Value
    else if Key = 'base' then
      Engine.BaseDirectory := Value
    else if Key = 'data' then
      Engine.DataDirectory := Value
    else if Key = 'port' then
      Engine.Port := Value
    else if Key = 'port-file' then
      Engine.PortFile := Value
    else if Key = 'listening' then
      Engine.Listening := Value
    else if Key = 'uninstaller' then
      Engine.Uninstaller := Value = 'yes'
    else if Key = 'unreadable' then
      Engine.Unreadable := Value
    else if Key = 'detail' then
      Engine.Detail := Value;
  end;
end;

// The probe dials localhost, as semibase does, so ::1 and 127.0.0.1 both count. Returns False
// when the probe itself failed, a constructor PowerShell refuses included. On an accepted port,
// Owner names each listening process as '<name> (pid <n>)'; empty when the lookup finds none.
function ProbePort(Port: Cardinal; var Accepts: Boolean; var Owner: String): Boolean;
var
  ExitCode: Integer;
  Output: TExecOutput;
begin
  Result := ExecAndCaptureOutput(PowerShellExe, '-NoProfile -NonInteractive -Command "try { ' +
    '([Net.Sockets.TcpClient]::new(''localhost'', ' + IntToStr(Port) + ')).Close() } catch { ' +
    'if ($_.Exception.InnerException -is [Net.Sockets.SocketException]) { exit 2 }; exit 3 }; ' +
    'try { Get-NetTCPConnection -LocalPort ' + IntToStr(Port) + ' -State Listen -ErrorAction ' +
    'SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object { ' +
    '''{0} (pid {1})'' -f (Get-Process -Id $_ -ErrorAction SilentlyContinue).ProcessName, $_ } ' +
    '} catch { }; exit 0"', '', SW_HIDE, ewWaitUntilTerminated, ExitCode, Output) and
    ((ExitCode = 0) or (ExitCode = 2));
  Accepts := ExitCode = 0;
  Owner := JoinLines(Output.StdOut);
end;

// The EDB uninstaller exits before it removes anything, and its copy in %TEMP% removes the
// registration, the service and the files over the next 14 to 70 s. A failed probe does not
// refuse: its line goes to the setup log, and Setup goes on.
function UninstallerRefusal: String;
var
  Started: Boolean;
  ExitCode: Integer;
  Output: TExecOutput;
  Failure: String;
begin
  Result := '';
  Failure := '';
  try
    Started := ExecAndCaptureOutput(PowerShellExe, '-NoProfile -NonInteractive -Command "try { ' +
      '$found = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -like ' +
      '''_uninstall*.000'' -or $_.Name -like ''uninstall-postgresql*'' }) } catch { exit 3 }; ' +
      'if ($found.Count -eq 0) { exit 2 }; ''{0} (pid {1})'' -f $found[0].Name, ' +
      '$found[0].ProcessId; exit 0"', '', SW_HIDE, ewWaitUntilTerminated, ExitCode, Output);
    if not Started then
      Failure := SysErrorMessage(ExitCode)
    else if ExitCode = 0 then
      Result := Refuse(CustomMessage('UninstallerRunning'),
        FmtMessage(CustomMessage('UninstallerRunningDetail'), [JoinLines(Output.StdOut)]))
    else if ExitCode <> 2 then
      Failure := 'exit code ' + IntToStr(ExitCode);
  except
    Failure := GetExceptionMessage;
  end;
  if Failure <> '' then
    AppendLine(LogPath('setup'), FmtMessage(CustomMessage('UninstallerProbeFailedDetail'), [
      Failure]));
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

// A port the server took from the default or from the service command line is in no file
function PortMismatchRefusal(const Engine: TEngine): String;
begin
  if Engine.PortFile = '' then
    Result := Refuse(FmtMessage(CustomMessage('PortMismatchNoFile'), [Engine.Listening,
      Engine.ServiceName, Engine.Port]), Engine.Detail)
  else
    Result := Refuse(FmtMessage(CustomMessage('PortMismatch'), [Engine.Listening,
      Engine.PortFile, Engine.Port, Engine.ServiceName]), Engine.Detail);
end;

// The refusals the service list alone decides, with no probe: the wizard pages show them.
function EngineStateRefusal(const Engine: TEngine): String;
begin
  Result := '';
  if Engine.ExecError <> '' then
    Result := FmtMessage(CustomMessage('SiteNotStarted'), [Engine.ExecError])
  else if Engine.State = StateAmbiguous then
    Result := Refuse(FmtMessage(CustomMessage('AmbiguousEngine'), [Engine.Services]),
      Engine.Detail)
  else if (Engine.State = StateIncomplete) and Engine.Uninstaller then
    Result := Refuse(FmtMessage(CustomMessage('IncompleteInstallation'), [Engine.ServiceName,
      Engine.BaseDirectory]), Engine.Detail)
  else if Engine.State = StateIncomplete then
    Result := Refuse(FmtMessage(CustomMessage('IncompleteInstallationNoUninstaller'), [
      Engine.ServiceName]), Engine.Detail)
  else if Engine.State = StateUnreadable then
    Result := Refuse(FmtMessage(CustomMessage('UnreadableService'), [Engine.ServiceName,
      Engine.Unreadable]), Engine.Detail)
  else if Engine.State = StatePortMismatch then
    Result := PortMismatchRefusal(Engine)
  else if Engine.State = StateOtherMajor then
    Result := Refuse(FmtMessage(CustomMessage('OtherMajor'), ['{#PgMajor}']), Engine.Detail)
  else if (Engine.State <> StateFound) and (Engine.State <> StateFresh) then
    Result := Refuse(CustomMessage('EngineFailed'), Engine.Detail);
end;

// No pg_ctl service runs: a fresh install must not meet a server or data it does not know.
function FreshMachineRefusal(const EngineDetail: String): String;
var
  Accepts: Boolean;
  Owner, OwnerDetail: String;
begin
  Result := '';
  if not ProbePort({#PgPort}, Accepts, Owner) then
    Result := FmtMessage(CustomMessage('ProbeFailed'), ['{#PgPort}'])
  else if Accepts then
  begin
    if Owner <> '' then
      OwnerDetail := FmtMessage(CustomMessage('PortOwnerDetail'), ['{#PgPort}', Owner])
    else
      OwnerDetail := FmtMessage(CustomMessage('PortNoOwnerDetail'), ['{#PgPort}']);
    Result := Refuse(FmtMessage(CustomMessage('PortBusy'), ['{#PgPort}']),
      EngineDetail + '; ' + OwnerDetail);
  end
  else if IsNonEmptyDirectory('{#PgDataDir}') then
    Result := Refuse(FmtMessage(CustomMessage('DataDirectoryNotEmpty'), ['{#PgDataDir}']),
      EngineDetail);
end;

function EngineRefusal(const Engine: TEngine; const SuperPassword: String): String;
begin
  Result := EngineStateRefusal(Engine);
  if (Result = '') and (Engine.State = StateFresh) then
  begin
    Result := FreshPasswordRefusal(SuperPassword);
    if Result = '' then
      Result := FreshMachineRefusal(Engine.Detail);
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
  Detail: String;
begin
  Result := '';
  ExitCode := ScExitCode('config "' + Engine.ServiceName + '" start= auto');
  if ExitCode <> 0 then
  begin
    Detail := FmtMessage(CustomMessage('ServiceConfigFailedDetail'), [Engine.ServiceName,
      IntToStr(ExitCode)]);
    Result := Refuse(FmtMessage(CustomMessage('ServiceConfigFailed'), [Engine.ServiceName,
      SysErrorMessage(ExitCode)]), Detail);
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

// semibase inherits the passwords from Setup's environment, so none is on a command line. All
// three variables are set even when empty, or a value inherited from the caller would reach
// semibase; semibase reads an empty one as unset and applies its convention, so semiplot gets
// the password semiplot the Ready and Finished pages name.
function SiteRefusal(const Engine: TEngine; const SuperPassword, WriterPassword: String): String;
var
  ExitCode: Integer;
begin
  Result := '';
  SetEnvironmentVariable('SEMIBASE_SUPER_PASSWORD', SuperPassword);
  SetEnvironmentVariable('SEMIBASE_WRITER_PASSWORD', WriterPassword);
  SetEnvironmentVariable('SEMIBASE_PLOT_PASSWORD', '');
  SiteLog := LogPath('site');
  SitePasswordRejected := False;
  if not ExecAndLogOutput(ExpandConstant('{tmp}\semibase.exe'), 'site --port ' +
    Engine.Port + ' --expected-major {#PgMajor} --restart-service "' + Engine.ServiceName + '"',
    '', SW_SHOWNORMAL, ewWaitUntilTerminated, ExitCode, @AppendSiteLine) then
    Result := FmtMessage(CustomMessage('SiteNotStarted'), [SysErrorMessage(ExitCode)])
  else if (ExitCode <> 0) and SitePasswordRejected then
    Result := Refuse(CustomMessage('SitePasswordWrong'),
      FmtMessage(CustomMessage('SitePasswordWrongDetail'), [SiteLog]))
  else if ExitCode <> 0 then
    Result := Refuse(FmtMessage(CustomMessage('SiteFailed'), [SiteLog]),
      FmtMessage(CustomMessage('SiteFailedDetail'), [IntToStr(ExitCode)]));
end;

var
  // What InitializeSetup's engine run found; every wizard page reads it, none runs the command
  Detected: TEngine;
  PasswordPage: TWizardPage;
  SuperEdit, WriterEdit: TNewEdit;
  Provisioned: TEngine;
  // The engine run finds the service once the install created it, so PrepareToInstall keeps its
  // answer
  ProvisionedFresh: Boolean;
  Refused: Boolean;
  ReadyRefusalLogged: Boolean;

// Returns True whatever the run found: a False here would end Setup with no exit code 7 and no
// setup log, and the Ready page and PrepareToInstall report a failed run instead
function InitializeSetup: Boolean;
begin
  DetectEngine(Detected);
  Result := True;
end;

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
  if Detected.State = StateFresh then
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
  // sure to do; on a found 17 semibase creates a missing user with the convention.
  if Detected.State = StateFresh then
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
begin
  Result := (PageID = PasswordPage.ID) and (EngineStateRefusal(Detected) <> '');
end;

// PrepareToInstall enforces every rule again; this only lets the user correct a field, so it
// skips the machine probes: a probe of a closed port takes about 7 s. A silent install simulates
// Next, and a False here would end it without exit code 7 or a setup log.
function NextButtonClick(CurPageID: Integer): Boolean;
var
  Refusal: String;
begin
  Result := True;
  if WizardSilent or (CurPageID <> PasswordPage.ID) then
    Exit;
  Refusal := WriterPasswordRefusal(WriterPasswordValue);
  if (Refusal = '') and (Detected.State = StateFresh) then
    Refusal := FreshPasswordRefusal(SuperPasswordOrConvention(SuperEdit.Text));
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

// The service list alone; PrepareToInstall decides. Inno runs this on every visit to the page
// and under /SILENT too, where PrepareToInstall copies the tool and logs the refusal anyway; a
// user who clicks Cancel as the page says never reaches PrepareToInstall, so the page does both
// once. The refusal stays the last log line, after a copy failure.
function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
var
  Refusal, ToolRefusal, Tools, WriterPassword: String;
begin
  Refusal := EngineStateRefusal(Detected);
  if Refusal <> '' then
  begin
    if not WizardSilent and not ReadyRefusalLogged then
    begin
      try
        ToolRefusal := KeepToolRefusal;
      except
        ToolRefusal := FmtMessage(CustomMessage('SetupError'), [GetExceptionMessage]);
      end;
      if ToolRefusal <> '' then
        AppendLine(LogPath('setup'), ToolRefusal);
      LogRefusal(Refusal);
      ReadyRefusalLogged := True;
    end;
    Result := FmtMessage(CustomMessage('ReadyRefusal'), [Refusal]);
    Exit;
  end;
  Tools := '{#AppName}' + NewLine +
    Space + FmtMessage(CustomMessage('ReadyTools'), [ExpandConstant('{#LogRoot}')]) + NewLine;
  WriterPassword := WriterPasswordOrConvention(WriterPasswordValue);
  if Detected.State = StateFresh then
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
  Result := FmtMessage(CustomMessage('ReadyPgFound'), ['{#PgMajor}']) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyFolder'), [Detected.BaseDirectory]) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyService'), [Detected.ServiceName]) + NewLine +
    Space + FmtMessage(CustomMessage('ReadyPort'), [Detected.Port]) + NewLine + NewLine +
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
  Fresh := False;
  try
    RefusalDetail := '';
    Result := KeepToolRefusal;
    if Result = '' then
      Result := WriterPasswordRefusal(WriterPassword);
    if Result = '' then
      Result := UninstallerRefusal;
    // The service list may have changed since InitializeSetup read it
    if Result = '' then
    begin
      DetectEngine(Engine);
      Fresh := Engine.State = StateFresh;
      Result := EngineRefusal(Engine, SuperPasswordOrConvention(SuperPassword));
    end;
    if (Result = '') and Fresh then
    begin
      Result := InstallEngineRefusal(SuperPasswordOrConvention(SuperPassword));
      if Result = '' then
      begin
        DetectEngine(Engine);
        if Engine.ExecError <> '' then
          Result := EngineStateRefusal(Engine)
        else if Engine.State <> StateFound then
          Result := Refuse(FmtMessage(CustomMessage('EdbFailed'), ['{#PgPrefix}']),
            Engine.Detail);
      end;
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
        '{#PgMajor}', Provisioned.Port, ExpandConstant('{#LogRoot}'),
        WriterPasswordOrConvention(WriterPasswordValue)])
    else
      WizardForm.FinishedLabel.Caption := FmtMessage(CustomMessage('FinishedText'), [
        '{#PgMajor}', Provisioned.Port, ExpandConstant('{#LogRoot}')]);
    WizardForm.AdjustLabelHeight(WizardForm.FinishedLabel);
  end;
end;
