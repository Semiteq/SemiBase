# SemiBase

![Go](https://img.shields.io/badge/Go-1.26-00ADD8)
[![Coverage Status](https://coveralls.io/repos/github/Semiteq/SemiBase/badge.svg?branch=master)](https://coveralls.io/github/Semiteq/SemiBase?branch=master)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-14+-336791)
![License](https://img.shields.io/badge/license-MIT-green)

<div align="center">
    <img src=./logo.png width=300 />
</div>

SemiBase — разворачиваемый PostgreSQL-сервис для установок Semiteq. Один экземпляр базы
данных, общий для приложений: Simple-Scada 2, [SemiPlot](https://github.com/Semiteq/SemiPlot) и других.

[Документация](./docs/readme.md)

## Требования

| Компонент  | Требование                                       |
| ---------- | ------------------------------------------------ |
| DB         | PostgreSQL 17. Установщик работает только с 17; утилита на тестовых стендах принимает 14 и новее |
| Writer     | Simple-Scada 2 с архивацией в PostgreSQL         |

Установщик `SemiBase-Setup.exe` — один файл на странице
[Releases](https://github.com/Semiteq/SemiBase/releases/latest). Он несёт PostgreSQL 17 и утилиту
`semibase.exe` и ничего не скачивает. Отдельного файла `semibase_<версия>_windows_amd64.exe` в
выпусках больше нет.

## Быстрое развёртывание

```powershell
# всё в PowerShell от имени администратора: установщик ставит PostgreSQL и управляет его службой
# пароли по соглашению: у каждой роли пароль совпадает с её именем, postgres/postgres,
# scada_writer/scada_writer, semiplot/semiplot; сервер принимает подключения только с этой машины.
# PostgreSQL 17, если его нет; конфигурация сервера, архивная база, роли `scada_writer`,
# `semiplot` и `semiplot_registrar`, права, таблица архива, пять таблиц SemiPlot и функция
# регистрации новых перьев; перезапуск службы, если настройки его ждут.
# код 0: машина готова; код 7: отказ, причина в C:\Program Files\SemiBase\Logs\setup-*.log
$setup = Start-Process -FilePath .\SemiBase-Setup.exe -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/LANG=russian' -Wait -PassThru
$setup.ExitCode
```

Как сменить пароль `postgres` или `scada_writer`, описано в
[руководстве по развёртыванию](./docs/deployment.md#пароли-по-соглашению).
