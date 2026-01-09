<div align="center">

# 📻 YggRadio

**Децентрализованная радиоплатформа в сети Yggdrasil**

[![Версия](https://img.shields.io/badge/версия-1.0.0-blue.svg)](https://github.com/JB-SelfCompany/yggradio/releases)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://go.dev/)
[![Лицензия](https://img.shields.io/badge/лицензия-GPLv3-green.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/JB-SelfCompany/yggradio/pulls)

*Самостоятельное размещение, приватность в приоритете, потоковое вещание через зашифрованную mesh-сеть Yggdrasil*

[Возможности](#-возможности) •
[Установка](#-установка) •
[Быстрый старт](#-быстрый-старт)

[🇬🇧 English version](README.md)

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

Скачайте последний релиз для вашей платформы:

```bash
# Linux/macOS
wget https://github.com/JB-SelfCompany/yggradio/releases/download/v1.0.0/yggradio-linux-amd64.tar.gz
tar -xzf yggradio-linux-amd64.tar.gz
sudo mv yggradio /usr/local/bin/

# Windows
# Скачайте со страницы релизов и добавьте в PATH
```

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

#### Сервис YggRadio

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

#### Сервис сервера федерации

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

YggRadio поддерживает **два метода аутентификации** - выберите тот, который подходит вам:

### Ключевые пары Ed25519

**Криптографическая аутентификация с фокусом на приватность, без паролей**

#### Ключи, сгенерированные в браузере (Быстро и просто)

1. Нажмите **"Войти"** → **"Создать новые ключи"**
2. **Сохраните ключи безопасно** (скачайте JSON файл)
3. Ключи хранятся в `sessionStorage` (очищаются при закрытии браузера)

#### Ключи, сгенерированные вручную (Максимальная безопасность)

Для максимальной безопасности генерируйте ключи вне браузера:

**Python (PyNaCl):**
```bash
python3 -c "import nacl.signing, base64; \
key = nacl.signing.SigningKey.generate(); \
print('Private:', base64.b64encode(bytes(key)).decode()); \
print('Public:', base64.b64encode(bytes(key.verify_key)).decode())"
```

**Node.js (tweetnacl):**
```bash
node -e "const nacl = require('tweetnacl'); \
const key = nacl.sign.keyPair(); \
console.log('Private:', Buffer.from(key.secretKey).toString('base64')); \
console.log('Public:', Buffer.from(key.publicKey).toString('base64'));"
```

**OpenSSL:**
```bash
openssl genpkey -algorithm ed25519 -out key.pem
openssl pkey -in key.pem -pubout -out pubkey.pem
# Примечание: Конвертируйте PEM в base64 вручную
```

**Импорт ключей:**
1. Нажмите **"Войти"** → **"Импортировать ключи"**
2. Вставьте ваши публичный и приватный ключи (формат base64)
3. Или загрузите JSON файл

**Безопасность:**
- ✅ Приватные ключи никогда не покидают ваше устройство
- ✅ Не нужно запоминать или хранить пароли
- ✅ Криптографические подписи для каждого запроса
- ✅ Автоматическая защита от replay-атак (5-минутное окно)

---

### Magic Link

**Простая аутентификация через закладку для легкого доступа**

#### Создание Magic Link

1. Нажмите **"Войти"** → **"Magic Link"**
2. Нажмите **"Сгенерировать Magic Link"**
3. Дождитесь вычисления Proof-of-Work (~2-4 секунды)
4. **Сохраните ссылку безопасно** (добавьте в закладки или скачайте)

#### Использование Magic Link

1. Перейдите по сохраненной ссылке в любом браузере
2. Автоматически создается session cookie (срок действия 1 неделя)
3. Заходите по ссылке в любое время для обновления сессии

**Заметки о безопасности:**
- ⚠️ **Любой, у кого есть ссылка, может получить доступ к вашему аккаунту** - храните её безопасно
- 🔒 Magic link никогда не истекает, но cookies истекают (1 неделя)
- 🔐 Токены и cookies хранятся как SHA256 хеши (192-бит и 256-бит энтропия)
- 🛡️ Constant-time сравнение предотвращает timing-атаки
- 📝 Рекомендуется: Храните в менеджере паролей или в закладках безопасно

**Когда использовать:**
- ✅ Быстрый доступ с нескольких устройств
- ✅ Не хотите управлять криптографическими ключами
- ✅ Предпочитаете аутентификацию в стиле закладок
- ❌ Требуются максимальные требования безопасности (используйте Ed25519)

---

## ⚙️ Конфигурация

Расположение файла конфигурации: `~/.yggradio/config.yaml`

При первом запуске YggRadio автоматически создает файл конфигурации по умолчанию. Вы также можете создать его вручную:

```bash
# Скопируйте пример конфигурации
cp config.example.yaml ~/.yggradio/config.yaml

# Отредактируйте конфигурацию
nano ~/.yggradio/config.yaml
```

**Основные настройки:**

```yaml
server:
  port: 8080
  bind: ""  # Автоопределение IPv6 адреса Yggdrasil
  instance_name: "Моё YggRadio"

streaming:
  max_listeners_per_station: 100
  max_source_clients: 10
  buffer_size: 32768
  server_secret: ""  # Автоматически генерируется при первом запуске

security:
  magic_link_enabled: true  # Включить аутентификацию magic link
  magic_link_token_length: 24  # 24 байта = 48 hex символов (192 бита)
  magic_link_cookie_ttl: 604800  # 1 неделя в секундах
  magic_link_require_pow: true  # Требовать PoW для защиты от спама
  magic_link_pow_difficulty: 16  # Такая же сложность как для создания станций

federation:
  enabled: false  # Установите в true для присоединения к федерации
  server_address: "301:be28:cf55:3c9::10"
  server_port: 9000
```

**Полный пример конфигурации:** См. [config.example.yaml](config.example.yaml)

### Конфигурация сервера федерации

Для запуска сервера федерации:

```bash
# Скопируйте пример конфигурации
cp config-federation.example.yaml ~/.yggradio-federation/config.yaml

# Отредактируйте конфигурацию
nano ~/.yggradio-federation/config.yaml
```

**Полный пример конфигурации федерации:** См. [config-federation.example.yaml](config-federation.example.yaml)

---

## 🎙️ Вещание

### Трансляция

Используйте любой клиент потокового вещания, поддерживающий HTTP стриминг:

**Пример с ffmpeg:**
```bash
ffmpeg -re -i music.mp3 -codec:a libmp3lame -b:a 128k \
  -f mp3 http://[ВАШ_YGGDRASIL_IP]:8080/your-mountpoint \
  -user username -password ваш-пароль-источника
```

**OBS Studio:**
1. Настройки → Вещание
2. Сервис: Пользовательский
3. Сервер: `http://[ВАШ_YGGDRASIL_IP]:8080/your-mountpoint`
4. Ключ потока: Используйте HTTP Basic Auth

**BUTT (Broadcast Using This Tool):**
1. Настройки → Сервер → Icecast
2. Адрес: Ваш IPv6 адрес Yggdrasil
3. Порт: 8080
4. Точка монтирования: /your-mountpoint

### Прослушивание

Откройте URL станции в вашем веб-браузере:
```
http://[YGGDRASIL_IP_ВЕЩАТЕЛЯ]:8080/
```

Веб-плеер автоматически загрузится и начнет воспроизведение.

---

## 🌐 Федерация

Федерация позволяет инстансам YggRadio находить друг друга через центральный хаб.

### Присоединение к федерации

1. Отредактируйте `~/.yggradio/config.yaml`:
   ```yaml
   federation:
     enabled: true
     server_address: "301:be28:cf55:3c9::10"  # Адрес сервера федерации
     server_port: 9000
   ```

2. Перезапустите YggRadio:
   ```bash
   sudo systemctl restart yggradio
   ```

### Запуск сервера федерации

```bash
# Установите сервер федерации
sudo cp yggradio-federation-server /usr/local/bin/

# Запустите через systemd
sudo cp systemd/yggradio-federation-server.service /etc/systemd/system/
sudo systemctl enable --now yggradio-federation-server
```

Конфигурация: `~/.yggradio-federation/config.yaml`

---

## 🏗️ Архитектура

### Автономный режим (по умолчанию)
Каждый экземпляр YggRadio работает независимо без необходимости во внешних сервисах:

```
┌─────────────────────────────────────────────────────────┐
│                    Сеть Yggdrasil                        │
│              (Зашифрованная Mesh-сеть)                   │
└─────────────────────────────────────────────────────────┘
                          │
    ┌─────────────────────┼─────────────────────┐
    │                     │                     │
┌───▼────┐          ┌─────▼─────┐        ┌─────▼─────┐
│Клиент  │          │ YggRadio  │        │ YggRadio  │
│Браузер │◄────────►│   Узел 1  │        │   Узел 2  │
└────────┘          │(Автономный)│        │(Автономный)│
                    └─────┬─────┘        └─────┬─────┘
                          │                    │
                          ▼                    ▼
                     ┌─────────┐          ┌─────────┐
                     │ SQLite  │          │ SQLite  │
                     │   БД    │          │   БД    │
                     └─────────┘          └─────────┘
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
    ┌────▼─────┐     ┌────▼─────┐    ┌────▼─────┐
    │YggRadio 1│◄───►│YggRadio 2│◄───►│YggRadio 3│
    │(Федератив)│     │(Федератив)│    │(Федератив)│
    └────┬─────┘     └────┬─────┘    └────┬─────┘
         │                │                │
    ┌────▼─────┐     ┌────▼─────┐    ┌────▼─────┐
    │ SQLite   │     │ SQLite   │    │ SQLite   │
    │   БД     │     │   БД     │    │   БД     │
    └──────────┘     └──────────┘    └──────────┘

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

### Структура проекта

```
yggradio/
├── cmd/
│   ├── yggradio/                      # Основное приложение
│   └── yggradio-federation-server/    # Сервер федерации
├── internal/
│   ├── api/
│   │   ├── handlers/                  # Обработчики HTTP запросов
│   │   └── middleware/                # Аутентификация, rate limiting, CSRF
│   ├── config/                        # Управление конфигурацией
│   ├── database/
│   │   ├── models/                    # Репозитории базы данных
│   │   └── schema.sql                 # Схема базы данных
│   ├── federation_client/             # Клиент федерации
│   ├── federation_server/             # Сервер федерации
│   ├── moderation/                    # PoW и фильтрация контента
│   ├── security/                      # Аутентификация, CSRF, валидация, санитизация
│   ├── streaming/                     # HTTP стриминговый сервер
│   ├── testutil/                      # Утилиты для тестирования
│   ├── utils/                         # Вспомогательные функции
│   └── web/
│       └── dist/                      # Встроенный фронтенд (собранный)
├── web/                               # Исходники React фронтенда
│   ├── src/
│   │   ├── components/                # React компоненты
│   │   ├── lib/                       # API клиент, утилиты
│   │   ├── pages/                     # Компоненты страниц
│   │   └── stores/                    # Хранилища состояния Zustand
│   └── dist/                          # Собранный фронтенд
├── systemd/                           # Файлы сервисов systemd
├── bin/                               # Скомпилированные бинарники (в gitignore)
└── dist/                              # Архивы релизов (в gitignore)
```

### Сборка

```bash
# Полная сборка (фронтенд + бэкенд для всех платформ)
bash build.sh

# Только бэкенд
go build -o bin/yggradio ./cmd/yggradio

# Только фронтенд
cd web && npm run build

# Dev-сервер фронтенда (с hot reload)
cd web && npm run dev

# Бэкенд с авто-перезагрузкой (требуется air)
air
```

### Тестирование

```bash
# Все тесты с race detector
go test -v -race ./...

# Только юнит-тесты (быстро)
go test -v -race -short ./...

# Тесты безопасности
go test -v -race ./internal/security/...

# Тесты фронтенда
cd web && npm test

# Отчет о покрытии
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Качество кода

```bash
# Форматирование
go fmt ./...

# Линтинг
go vet ./...
cd web && npm run lint

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