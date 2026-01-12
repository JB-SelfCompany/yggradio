<div align="center">

# 📻 YggRadio

**Децентрализованная радиоплатформа в сети Yggdrasil**

[![Версия](https://img.shields.io/badge/версия-1.1.0-blue.svg)](https://github.com/JB-SelfCompany/yggradio/releases)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://go.dev/)
[![Лицензия](https://img.shields.io/badge/лицензия-GPLv3-green.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/JB-SelfCompany/yggradio/pulls)

*Самостоятельное размещение, приватность в приоритете, потоковое вещание через зашифрованную mesh-сеть Yggdrasil*

[Возможности](#-возможности) •
[Установка](#-установка) •
[Быстрый старт](#-быстрый-старт) •
[🇬🇧 English](README.md)

</div>

---

## ✨ Возможности

- 🔒 **Сквозное шифрование** - Весь трафик автоматически шифруется через Yggdrasil (TLS 1.3)
- 🚫 **Без центральных серверов** - Полностью децентрализованная P2P архитектура
- 🎵 **Мультиформатное вещание** - Поддержка MP3, Ogg Vorbis, Opus, AAC, FLAC
- 🔐 **Двойная аутентификация** - Ed25519 подписи ИЛИ magic link аутентификация
- 🔑 **Приватность прежде всего** - Не требуется username, email или личные данные
- 🌐 **Поддержка федерации** - Опциональная hub-and-spoke федерация для обнаружения станций
- 📦 **Единый бинарный файл** - Нет внешних зависимостей кроме демона Yggdrasil
- 🎨 **Современный веб-интерфейс** - React-based, адаптивный интерфейс
- 🔥 **Слушатели в реальном времени** - Счетчик слушателей и статистика станций в реальном времени
- 🛡️ **Усиленная безопасность** - Rate limiting, защита от CSRF, предотвращение XSS
- ⚡ **Низкая задержка** - Оптимизированные буферы для потоковой передачи с минимальной задержкой

---

## 📋 Содержание

- [Требования](#-требования)
- [Установка](#-установка)
  - [Из бинарного файла](#из-бинарного-файла)
  - [Из исходного кода](#из-исходного-кода)
  - [Использование Systemd](#использование-systemd)
- [Быстрый старт](#-быстрый-старт)
- [Аутентификация](#-аутентификация)
  - [Ключевые пары Ed25519](#ключевые-пары-ed25519)
  - [Magic Link](#magic-link)
- [Конфигурация](#-конфигурация)
- [Вещание](#-вещание)
  - [Трансляция](#трансляция)
  - [Прослушивание](#прослушивание)
- [Федерация](#-федерация)
- [Архитектура](#-архитектура)
- [Разработка](#-разработка)
- [Лицензия](#-лицензия)
- [Поддержка](#-поддержка)

---

## 🔧 Требования

- **Демон Yggdrasil** запущенный на той же машине ([Руководство по установке](https://yggdrasil-network.github.io/installation.html))
- **Go 1.21+** (для сборки из исходников)
- **Node.js** (только для разработки фронтенда)

---

## 📦 Установка

### Из бинарного файла

**[Скачать с GitHub Releases](https://github.com/JB-SelfCompany/yggradio/releases/latest)**

Выберите бинарный файл для вашей платформы (Linux, macOS или Windows), распакуйте его и добавьте в PATH.

### Из исходного кода

```bash
# Клонируйте репозиторий
git clone https://github.com/JB-SelfCompany/yggradio.git
cd yggradio

# Сборка (включая фронтенд)
bash build.sh  # Linux/macOS

# Бинарный файл будет в bin/yggradio
```

### Использование Systemd

Для production развертывания на Linux используйте systemd для запуска YggRadio как сервиса:

#### Сервис YggRadio (Обязательный)

**Это основное приложение, которое вам нужно запустить.**

```bash
# Создайте системного пользователя
sudo useradd -r -s /bin/false yggradio

# Скопируйте бинарный файл и сделайте исполняемым
sudo cp bin/yggradio-linux-amd64 /usr/local/bin/yggradio-linux
sudo chmod +x /usr/local/bin/yggradio-linux

# Создайте директорию конфигурации
sudo mkdir -p /home/yggradio/.yggradio
sudo cp config.example.yaml /home/yggradio/.yggradio/config.yaml
sudo chown -R yggradio:yggradio /home/yggradio
# При необходимости отредактируйте: sudo nano /home/yggradio/.yggradio/config.yaml

# Скопируйте файл сервиса systemd
sudo cp systemd/yggradio.service /etc/systemd/system/

# Перезагрузите systemd
sudo systemctl daemon-reload

# Включите сервис (автозапуск при загрузке)
sudo systemctl enable yggradio

# Запустите сервис
sudo systemctl start yggradio

# Проверьте статус
sudo systemctl status yggradio

# Просмотр логов
sudo journalctl -u yggradio -f
```

#### Сервис сервера федерации (Опционально - Только для продвинутых пользователей)

> **⚠️ Примечание:** Этот сервис **опционален** и нужен только если вы хотите запустить собственный хаб обнаружения федерации. Большинству пользователей следует **пропустить этот раздел** и запускать только основной сервис YggRadio выше. Вы можете присоединиться к существующей федерации, настроив `federation.enabled: true` в вашем `config.yaml` без запуска сервера федерации.

```bash
# Создайте системного пользователя
sudo useradd -r -s /bin/false yggradio-federation

# Скопируйте бинарный файл и сделайте исполняемым
sudo cp bin/yggradio-federation-server-linux-amd64 /usr/local/bin/yggradio-federation-server
sudo chmod +x /usr/local/bin/yggradio-federation-server

# Создайте директорию конфигурации
sudo mkdir -p /home/yggradio-federation/.yggradio-federation
sudo cp config-federation.example.yaml /home/yggradio-federation/.yggradio-federation/config.yaml
sudo chown -R yggradio-federation:yggradio-federation /home/yggradio-federation
# При необходимости отредактируйте: sudo nano /home/yggradio-federation/.yggradio-federation/config.yaml

# Скопируйте файл сервиса systemd
sudo cp systemd/yggradio-federation-server.service /etc/systemd/system/

# Перезагрузите systemd
sudo systemctl daemon-reload

# Включите сервис (автозапуск при загрузке)
sudo systemctl enable yggradio-federation-server

# Запустите сервис
sudo systemctl start yggradio-federation-server

# Проверьте статус
sudo systemctl status yggradio-federation-server

# Просмотр логов
sudo journalctl -u yggradio-federation-server -f
```

**Примечание:** Оба сервиса работают под выделенными системными пользователями с включенным усилением безопасности (NoNewPrivileges, ProtectSystem, PrivateTmp). Файлы конфигурации хранятся в домашнем каталоге пользователя (`~/.yggradio/` или `~/.yggradio-federation/`).

---

## 🚀 Быстрый старт

> **📝 Примечание:** Это руководство показывает, как запустить основное приложение YggRadio. Вам **не нужно** запускать сервер федерации - YggRadio отлично работает в автономном режиме!

1. **Установите и запустите демон Yggdrasil:**
   ```bash
   # См.: https://yggdrasil-network.github.io/installation.html
   sudo systemctl start yggdrasil
   ```

2. **Запустите YggRadio:**
   ```bash
   yggradio
   ```

3. **Откройте веб-интерфейс:**
   - YggRadio отобразит свой URL при запуске (например, `http://[200:xxxx:xxxx:xxxx::1]:8080`)
   - Откройте этот URL в браузере с любого устройства, подключенного к Yggdrasil

4. **Создайте вашу первую станцию:**
   - Нажмите "Create Station" в веб-интерфейсе
   - Начните вещание с помощью ffmpeg, OBS или BUTT

---

## 🔐 Аутентификация

YggRadio поддерживает **два метода аутентификации**:

### Ключевые пары Ed25519

**Криптографическая аутентификация с фокусом на приватность, без паролей**

- Нажмите **"Войти"** → **"Создать новые ключи"** или **"Импортировать ключи"**
- Ключи хранятся в `sessionStorage` браузера (очищаются при закрытии)
- Для максимальной безопасности генерируйте ключи вне браузера через PyNaCl, tweetnacl или OpenSSL

**Безопасность:**
- ✅ Приватные ключи никогда не покидают ваше устройство
- ✅ Пароли не требуются
- ✅ Криптографические подписи с автоматической защитой от replay-атак

### Magic Link

**Простая аутентификация через закладку для легкого доступа**

1. Нажмите **"Войти"** → **"Magic Link"** → **"Сгенерировать Magic Link"**
2. Дождитесь вычисления Proof-of-Work (~2-4 секунды)
3. Сохраните ссылку безопасно (закладка или менеджер паролей)
4. Переходите по ссылке в любое время для аутентификации

**Заметки о безопасности:**
- ⚠️ Любой со ссылкой может получить доступ к аккаунту
- 🔒 Ссылка не истекает, cookies истекают через 1 неделю
- ✅ Лучше для быстрого доступа с нескольких устройств
- ❌ Для максимальной безопасности используйте Ed25519

---

## ⚙️ Конфигурация

Файл конфигурации: `~/.yggradio/config.yaml` (создается автоматически при первом запуске)

**Основные настройки:**
- `server.port`: Порт веб-интерфейса (по умолчанию: 8080)
- `server.bind`: IPv6 адрес (определяется автоматически)
- `streaming.max_listeners_per_station`: Лимит слушателей (по умолчанию: 100)
- `security.magic_link_enabled`: Включить magic link аутентификацию (по умолчанию: true)
- `federation.enabled`: Присоединиться к сети федерации (по умолчанию: false)

**Полные примеры конфигурации:** [config.example.yaml](config.example.yaml), [config-federation.example.yaml](config-federation.example.yaml)

---

## 🎙️ Вещание

### Трансляция

Используйте любой HTTP стриминг клиент (ffmpeg, OBS Studio, BUTT):

**Пример с ffmpeg:**
```bash
ffmpeg -re -i audio.flac -codec:a libmp3lame -b:a 128k \
  -f mp3 -content_type audio/mpeg -method PUT \
  'http://username:password@[ВАШ_IP]:8080/mountpoint'
```

### Прослушивание

Откройте `http://[IP_ВЕЩАТЕЛЯ]:8080/` в браузере - веб-плеер загрузится автоматически.

---

## 🌐 Федерация

**Опционально** - YggRadio отлично работает автономно. Включайте федерацию только для обнаружения станций от других узлов.

**Присоединение к федерации:** Отредактируйте `~/.yggradio/config.yaml`:
```yaml
federation:
  enabled: true
  server_address: "301:be28:cf55:3c9::10"
  server_port: 9000
```

**Запуск собственного сервера федерации:** См. файлы сервисов systemd и `config-federation.example.yaml`

---

## 🏗️ Архитектура

### Автономный режим (по умолчанию)
Каждый экземпляр YggRadio работает независимо без необходимости во внешних сервисах:

```
┌─────────────────────────────────────────────────────────┐
│                    Сеть Yggdrasil                       │
│              (Зашифрованная Mesh-сеть)                  │
└─────────────────────────────────────────────────────────┘
                          │
    ┌─────────────────────┼─────────────────────┐
    │                     │                     │
┌───▼────┐          ┌─────▼──────┐        ┌─────▼──────┐
│Клиент  │          │  YggRadio  │        │  YggRadio  │
│Браузер │◄────────►│   Узел 1   │        │   Узел 2   │
└────────┘          │(Автономный)│        │(Автономный)│
                    └─────┬──────┘        └─────┬──────┘
                          │                     │
                          ▼                     ▼
                     ┌─────────┐           ┌─────────┐
                     │ SQLite  │           │ SQLite  │
                     │   БД    │           │   БД    │
                     └─────────┘           └─────────┘
```

### Режим федерации (опционально)
Включите федерацию для автоматического обнаружения станций в сети:

```
                 ┌──────────────────────────┐
                 │   Сервер федерации       │
                 │ (Опциональное открытие)  │
                 │  - Реестр станций        │
                 │  - Heartbeat узлов       │
                 └────────┬─────────────────┘
                          │
         ┌────────────────┼────────────────┐
         │                │                │
    ┌────▼──────┐     ┌────▼──────┐     ┌────▼──────┐
    │YggRadio 1 │◄───►│YggRadio 2 │◄───►│YggRadio 3 │
    │(Федератив)│     │(Федератив)│     │(Федератив)│
    └────┬──────┘     └────┬──────┘     └────┬──────┘
         │                 │                 │
    ┌────▼─────┐      ┌────▼─────┐      ┌────▼─────┐
    │ SQLite   │      │ SQLite   │      │ SQLite   │
    │   БД     │      │   БД     │      │   БД     │
    └──────────┘      └──────────┘      └──────────┘

    Узел 1 ◄──────► Узел 2 ◄──────► Узел 3
    (Прямой обмен станциями между узлами)
```

**Варианты развертывания:**
- **Автономный**: Каждый экземпляр работает независимо (по умолчанию)
- **Федеративный**: Узлы регистрируются на сервере федерации для обнаружения в сети
- **Гибридный**: Сервер федерации на том же хосте с использованием `localhost` для нулевой задержки

**Ключевые компоненты:**

- **Интеграция с Yggdrasil**: Автоопределение IPv6 адреса, весь трафик шифруется
- **HTTP Streaming сервер**: Принимает клиентов-источников и обслуживает слушателей
- **Веб-интерфейс**: React + TypeScript фронтенд (встроен в бинарник)
- **База данных**: SQLite для станций, пользователей, метаданных (минималистичная схема с фокусом на приватность)
- **Двойная аутентификация**: Подписи Ed25519 ИЛИ magic link + session cookies
- **Слой безопасности**: Защита от CSRF, rate limiting, предотвращение XSS, логирование аудита
- **Клиент федерации**: Опциональное обнаружение через хаб федерации (по умолчанию отключен)

**Функции приватности:**
- ✅ Не требуется имена пользователей, email или личная информация
- ✅ Приватные ключи Ed25519 никогда не покидают ваше устройство
- ✅ Токены magic link и cookies хранятся только как SHA256 хеши
- ✅ Минималистичная схема БД (id, pubkey/null, временные метки)
- ✅ Хранение ключей только на время сессии (sessionStorage, не localStorage)
- ✅ IP-адреса не сохраняются в audit логах (приватное логирование)

---

## 🛠️ Разработка

### Сборка

```bash
bash build.sh                          # Полная сборка (все платформы)
go build -o bin/yggradio ./cmd/yggradio  # Только бэкенд
cd web && npm run build                # Только фронтенд
cd web && npm run dev                  # Dev-сервер с hot reload
```

### Тестирование

```bash
go test -v -race ./...                 # Все тесты
go test -v -race -short ./...          # Только юнит-тесты
cd web && npm test                     # Тесты фронтенда
```

---

## 📄 Лицензия

Этот проект лицензирован под GNU General Public License v3.0 - см. файл [LICENSE](LICENSE) для деталей.

YggRadio - это свободное программное обеспечение: вы можете распространять и/или модифицировать его в соответствии с условиями GNU General Public License, опубликованной Free Software Foundation, либо версии 3 Лицензии, либо (по вашему выбору) любой более поздней версии.

---

## 💬 Поддержка

- **Issues**: [GitHub Issues](https://github.com/JB-SelfCompany/yggradio/issues)
- **Обсуждения**: [GitHub Discussions](https://github.com/JB-SelfCompany/yggradio/discussions)
- **Сеть Yggdrasil**: [yggdrasil-network.github.io](https://yggdrasil-network.github.io/)

---

## 🙏 Благодарности

- [Yggdrasil Network](https://yggdrasil-network.github.io/) - Зашифрованная mesh-сеть
- [modernc.org/sqlite](https://modernc.org/sqlite) - Pure Go SQLite

---

<div align="center">

**Сделано с ❤️ для децентрализованного веба**

⭐ Поставьте звезду на GitHub — это помогает!

</div>