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

Утилита развёртывания — один исполняемый файл, среда исполнения не требуется.

## Быстрое развёртывание

```powershell
# установка pgsql
winget install --id PostgreSQL.PostgreSQL.17 --exact

# конфигурация сервера, архивная база, роли, права, таблица архива
.\semibase.exe site
```
