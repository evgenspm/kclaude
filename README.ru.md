# kclaude

**Claude Code с Opus 5.5 через ваши аккаунты Kiro.**

Добавьте API-ключи Kiro и запускайте `kclaude` в папке проекта. Получите установленный у вас Claude Code с его инструментами и отдельным профилем. При отказе одного аккаунта из-за лимита или авторизации роутер попробует следующий.

[English and full reference](README.md)

## Установка

Нужны macOS или Linux, Python 3.9+, `curl` и [Claude Code](https://code.claude.com/docs/en/setup), доступный командой `claude`. Windows поддерживается через WSL2. Готовые сборки: arm64 и amd64. Go и Kiro CLI для установки не нужны.

```sh
curl -fsSL https://raw.githubusercontent.com/evgenspm/kclaude/main/install.sh | sh
```

Команда появится в `~/.local/bin/kclaude`. Если этой папки нет в `PATH`, используйте полный путь или добавьте её в настройки своего shell.

## Подключение аккаунтов

Откройте [app.kiro.dev](https://app.kiro.dev/) → **API Keys** и создайте ключ. Kiro предоставляет API-ключи аккаунтам Pro, Pro+, Pro Max и Power; запросы расходуют кредиты соответствующей подписки. [Инструкция Kiro](https://kiro.dev/docs/getting-started/authentication/#api-key-authentication-cli).

```sh
kclaude accounts add personal
kclaude accounts add work
kclaude accounts list
```

Каждая команда `add` попросит вставить ключ `ksk_…` со скрытым вводом. Добавляйте свои аккаунты или аккаунты, которые вам разрешено использовать. Можно прочитать ключ из файла: `kclaude accounts add work --key-file /path/to/key`. Для EU-аккаунта добавьте `--region eu-central-1`; по умолчанию используется `us-east-1`.

## Запуск

```sh
cd your-project
kclaude
```

По умолчанию: **Opus 5.5, контекст 1M и `--dangerously-skip-permissions`**. Claude сможет выполнять команды и менять файлы без подтверждений. Для обычных запросов разрешений запускайте `kclaude --safe`.

```sh
kclaude --safe
kclaude --model sonnet
kclaude --seat work
kclaude --continue
kclaude -p "Объясни этот репозиторий"
```

`sonnet` выбирает Sonnet 5.5, `haiku` выбирает Haiku 4.5. Доступ к модели зависит от вашего аккаунта. `kclaude models` показывает список моделей адаптера, а не остаток кредитов или права аккаунта.

## Продолжить чат обычного Claude

В той же папке проекта:

```sh
kclaude --from-claude
```

Или укажите Session ID из `/status`:

```sh
kclaude --from-claude-id YOUR_SESSION_UUID
```

Команда скопирует историю и папку с дополнительными данными сессии, затем продолжит чат через `--resume --fork-session`. Оригинальная история сохранится. Фоновые процессы, глобальные плагины, хуки и файлы памяти не переносятся. Файлы проекта остаются общими: перед переносом прекратите правки в исходном чате.

## Управление

```sh
kclaude status
kclaude accounts disable work
kclaude accounts enable work
kclaude accounts remove work
kclaude stop
```

Меняйте список аккаунтов между запросами: изменение остановит роутер, чтобы он перечитал ключи при следующем запуске. При HTTP 401/402/403/429 роутер временно откладывает аккаунт и пробует следующий. Он учитывает `Retry-After` для лимитов. Уже начавшийся успешный ответ и неоднозначную сетевую ошибку он не переигрывает. Если доступных аккаунтов нет, вернётся ошибка. WebSearch использует доступный аккаунт без переключения после ошибки.

## Где хранятся данные

Ключи, настройки и история лежат в `~/.local/share/kclaude/`. Ключи имеют права `0600`. Профиль Claude: `~/.local/share/kclaude/claude/`. Лог роутера: `~/.local/share/kclaude/router.log`.

Установщик не меняет обычный `claude`, `~/.claude` и настройки shell. Роутер слушает только `https://127.0.0.1:17391`, проверяет локальный токен и отклоняет запросы браузера с Origin. Локальный сертификат доверяется только дочерним процессом Claude; системное хранилище сертификатов не меняется. Запросы к модели отправляются в Kiro с вашими ключами.

Для обновления повторите команду установки. Затем между сессиями выполните `kclaude stop`: при следующем запуске поднимется новая версия. Ключи и история сохранятся.

Это независимый неофициальный адаптер. Совместимость зависит от изменений Kiro и Claude Code. Claude Code нужно установить отдельно. Лицензия Apache-2.0; исходный адаптер основан на [ClaudeCode Kiro](https://github.com/itututu/claudecode-kiro) и [kirocc](https://github.com/d-kuro/kirocc). Подробности, настройки путей и удаление: [README](README.md).
