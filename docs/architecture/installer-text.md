# Installer texts

The rules every text of `SemiBase-Setup.exe` follows: the wizard pages, field labels and hints,
the message boxes, the Ready page, the Finished page, the uninstaller texts, and the refusal line
a headless run leaves in its setup log. The texts are the `[Messages]` and `[CustomMessages]`
entries of `Installer/SemiBase.iss`;
[installer.md](./installer.md#the-page-the-folder-and-the-texts) describes where each one shows. Russian is the primary language; the English text says the same,
rule for rule.

The reader is a commissioning engineer at an industrial site. They know Windows, the SCADA and
its project settings; they do not know this program's internals; they are often in a hurry; they
may have no internet; they may run Setup headless and then see only the last log line.

Each rule names the operator's objection it comes from, quoted as the operator wrote it, so a
later edit can check the rule is still needed. The numbers are stable: code notes and reviews
cite them.

## Words

1. **Formal register.** No colloquial verbs or idioms: not «подходят», «ломает», «проходит»,
   «так требует», «не знает». Use the neutral verb: «можно оставить», «нарушает»,
   «соответствует», «это ограничение…», «не зарегистрирован». Nor the opposite: no канцелярит
   («выполнить удаление», «осуществить», «данный»); write «удалите», «этот». (Objections:
   «подходят — просторечье», «ломает — просторечье», «так требует, проходит — просторечье», «не
   знает — просторечье».)
2. **Software does not think or feel.** A program does not «know», «want», «wait for» or
   «understand». State the observable condition instead. (Objections: «не знает — просторечье»;
   the restart wording, which rule 10 owns.)
3. **The reader's own terms.** Use the word the reader meets in the tools they configure; take
   it from that tool's manual. The SCADA manual says «пользователь БД»; this program's internal
   word «роль» never reaches the screen. (Objection: «что такое роль из контекста понять
   невозможно — посмотри в документации скады какой там термин».)
4. **One term per concept.** Three things get three fixed terms: the server is «PostgreSQL 17»,
   or «PostgreSQL» once the page or the sentence has said 17; the database is «база
   scada_archive»; an account is «пользователь БД <name>». «БД» appears only inside the last two
   terms. A folder is «папка», a log «журнал», this program «установщик», the EDB program
   «установщик PostgreSQL» («установщик SemiBase» only in a sentence that names both), the
   binary «утилита semibase.exe», the start mode «тип запуска «Автоматически»». Never alternate
   synonyms. (Objection: «где-то говорится БД, где-то PostgreSQL».)
5. **Name by role, identifier in brackets.** Say what a thing is, then its technical name:
   «пароль администратора PostgreSQL (пользователь БД postgres)», not «пароль postgres». A bare
   identifier is a label only where the role is already said nearby. (Objection: «postgres —
   неформальное название».)
6. **No ambiguous pronouns.** A pronoun whose noun is not in the same sentence is replaced by the
   noun. «С ним SCADA пишет архив» becomes «С этим паролем SCADA записывает архив». (Objection:
   «с ним — с чем?».)

## Content

7. **Facts only, no guesses.** State what the program observed; never a possible cause it did
   not check («возможно, антивирус», «вероятно, …»). If the cause is unknown, give the observed
   error text and the action. (Objection: «assumption про антивирус, лучше убрать», three times.)
8. **No outside parties.** Do not send the reader to support, a vendor or a person. The text
   gives the action the reader can take. (Objection: «не нужно упоминать поддержку».)
9. **No pointers to what is not on the machine.** The text carries its own action. It may name a
   file on this machine by its exact path (a log, the uninstaller); it may not name a document, a
   site or a section unless that document is installed at a fixed path the text gives. No guide
   is installed, so there are no pointers at all. (Objections: «какое руководство», «ссылки на
   гитхаб собирались удалять?».)
10. **Conditions as the reader observes them.** A conditional says when, in terms the reader can
    check: «при первой настройке этого сервера», «если архив на нём не нужен»; never an internal
    state («если настройки этого ждут», «если изменились настройки сервера»). When the reader's
    action is the same in every branch, drop the condition and give the action. (Objection: the
    restart wording.)
11. **Say what will happen and where a value comes from.** The page before an irreversible step
    lists what changes, which values are used and where each came from, and what the reader must
    do first (for example, stop the SCADA). A value whose purpose is not obvious says what it is
    for («порт 5432 — к нему подключаются SCADA и SemiPlot»). Rule 25 limits "where it came
    from" to the reader's terms. (Objection: «покрасивее отредактировать выводимый текст чтобы
    было понятно что сейчас будет устанавливаться и что это за порт для чего он и откуда
    взялся».)

## Structure

12. **Each explanation next to its control.** A hint sits under the field it explains, not in a
    general paragraph above all fields. (Objection: «перед полем должно быть описание конкретно
    к нему а не общее в начале».)
13. **Show a control only when it means one thing.** A field whose meaning depends on hidden
    state («пусто — создать с паролем X, иначе не менять») is not shown; show it only in the
    state where it has one meaning. (Objections: «поведение странное пусто → задает пароль
    scada_writer»; «можно ли отобразить окно ввода пароля только тогда когда оно нужно чтобы не
    путать юзера».)
14. **Refusal pattern: what happened, then what to do.** One sentence states the observed
    problem in the reader's terms; the next gives the action. Technical detail (a registry key,
    an exit code) goes to the setup log line before the refusal (rule 25). No reasons the reader
    cannot act on (code pages, command-line parsing). The refusal blames nobody: «Неверный пароль
    …», not «Вы ввели неверный пароль». Refusals that share one recovery share its steps word for
    word: every text that sends the reader to the PostgreSQL uninstaller says to wait until the
    removal ends.
15. **Lists, one item per line.** Several facts are a list with one item per line, never a long
    sentence that a fixed-width window cuts. (Objection: the Ready page cut at the window edge.)
16. **Length.** As short as the facts and the action allow, and no shorter: a hint is one or two
    sentences; a refusal is at most two sentences (problem, action); a summary line is one fact.
    Cut, in this order: a noun the page heading or the previous sentence already fixed (repeat it
    only where rule 6 needs it); a qualifier that distinguishes nothing in this sentence
    («установщик SemiBase» where no other installer is named, «Windows» after «служба»); a reason
    the reader does not act on; a parenthesis that repeats the sentence. Length is judged against
    rules 11, 14 and 18, not against an earlier text: a value the reader types, a step the
    recovery needs, or an exact path is never cut for length.
17. **A refusal is one line.** Headless runs and the log keep only the last line, so a refusal
    has no line breaks and no lists; rule 15 applies to pages and dialogs only.

## Form

18. **Exact values.** Values the reader types or compares are written exactly: paths, names,
    ports, menu paths («Проект → Настройки → База данных»). Windows UI names are written as the
    target Windows writes them («Программы и компоненты»), verified on that Windows.
19. **Mood and order.** Imperative for actions («Удалите…», «Запустите установщик снова»),
    present tense for facts. A condition comes before its command: «Если службы %1 нет,
    удалите…».
20. **Quotes.** Russian quotes «…» for quoted UI text; no nested quotes; one sentence per line in
    a list item.
21. **Language.** The interface follows the language the reader chose; the English text is
    equivalent, not a loose translation.
22. **«вы» lowercase, «ё» everywhere.** The texts write «ё» («удалён», «задаёт»); Inno's stock
    strings mix both and are not changed.

## Precision and placement

23. **No permissive or vague modals.** A technical text states what is or what to do; it does not
    grant permission or hedge: no «можно», «возможно», «может быть», «при необходимости» where
    the program's behaviour is fixed. «Значения в полях можно оставить» becomes a statement of
    what the fields hold. (Objection: «можно — плохая формулировка. в техническом руководстве
    слишком вольная формулировка».)
24. **Behaviour is stated as determined.** When the program does a thing under a condition, the
    text gives the condition; it never says the thing «is possible». If the condition cannot be
    put in the reader's terms, name the observable effect and when it happens. (Objection: «что
    значит возможен? поведение неопределено явно».)
25. **No storage internals on screen; they go to the log.** The registry, its keys, exit codes
    and other internal stores are not on a page or in a dialog. The screen says what was found or
    not found in the reader's terms («установленный PostgreSQL 17 не найден»); the setup log line
    before the refusal carries the technical detail for whoever investigates. This narrows rules
    11 and 14: "where a value comes from" is said in the reader's terms («из установленного
    PostgreSQL 17»), and a refusal's technical detail goes to the log only. (Objections: «реестр
    лишняя сущность для оператора … его упоминание не внесет ясности», «эта информация допустима
    в логах».)
26. **A path the reader must use is copyable or clickable.** A path or command the reader is told
    to open, run or delete is shown where it can be selected and copied (a read-only text field),
    or behind a button that opens it. Static label text is not enough. (Objection: «если
    оператору предлагается путь он должен быть или кликабельным или копируемым».)
27. **Plain clauses, no participle chains.** Say a relation with a verb in its own clause, not
    with a participle or an adjective hung on a noun («пароль по умолчанию, равный имени
    пользователя» becomes «пароль по умолчанию: имя пользователя»). (Objection: «пароль … равный
    звучит криво».)

## Where the rules are met in code

- Rule 25: a refusal function returns `Refuse(Text, Detail)`, which keeps `Detail` in the
  `RefusalDetail` global; `LogRefusal` writes it on the line before the refusal. Detail entries
  are `[CustomMessages]` without a language prefix, English in every language.
- Rule 26: the Ready page is a read-only `ReadyMemo`; a refusal on the Preparing page moves from
  the static `PreparingLabel` into the read-only `PreparingMemo` (`ShowRefusalAsText`). The two
  message boxes the Next button raises hold no path.
- Open checks: the bold superuser label and the 15-line Finished text at 150 % DPI, the
  Preparing memo with the longest refusal at 150 % DPI, the English SCADA menu path against an
  English Simple-Scada, and «Программы и компоненты» on the target Windows (rule 18).
