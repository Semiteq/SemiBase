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
| DB         | PostgreSQL 17; минимально поддерживаемая — 14    |
| Writer     | Simple-Scada 2 с архивацией в PostgreSQL         |

Установщик `SemiBase-Setup.exe` — один файл на странице
[Releases](https://github.com/Semiteq/SemiBase/releases/latest). Он несёт PostgreSQL 17 и ничего не
скачивает.

## Быстрое развёртывание

```powershell
# всё в PowerShell от имени администратора: установщик ставит PostgreSQL и управляет его службой
# пароли ролей: каждая строка запрашивает пароль, в историю PowerShell он не попадает
$env:SEMIBASE_SUPER_PASSWORD = [Net.NetworkCredential]::new('', (Read-Host 'Пароль postgres' -AsSecureString)).Password
$env:SEMIBASE_WRITER_PASSWORD = [Net.NetworkCredential]::new('', (Read-Host 'Пароль scada_writer' -AsSecureString)).Password

# PostgreSQL 17, если его нет; конфигурация сервера, архивная база, роли `scada_writer`,
# `semiplot` (пароль `semiplot`) и `semiplot_registrar`, права, таблица архива, пять таблиц
# SemiPlot и функция регистрации новых перьев; перезапуск службы, если настройки его ждут.
# код 0: машина готова; код 7: отказ, причина в C:\DISTR\Logs\SemiBase\setup-*.log
$setup = Start-Process -FilePath .\SemiBase-Setup.exe -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART' -Wait -PassThru
$setup.ExitCode
```
