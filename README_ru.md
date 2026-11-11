# TeraBox CLI (tbc)

![tbc_logo](https://repository-images.githubusercontent.com/963345828/78ea40a2-977c-4de4-b24d-74d9f3cef4c6)

[English](README.md) · **Русский** · [日本語](README_ja.md)

Простой и мощный клиент командной строки для TeraBox, написанный на Go.
Поддерживает вывод списка файлов в стиле `ls`, экономичную по памяти разделяемую загрузку/скачивание и интерактивный режим (REPL).

## ✨ Возможности

- **Привычный Unix-подход**: знакомые команды `ls`, `cp`, `mv`, `rm`.
- **Богатый вывод**: команда `ls` поддерживает ANSI-цвета и сеточное отображение с автоматической подстройкой под ширину терминала.
- **Экономия памяти**: потоковая обработка сводит потребление памяти к минимуму даже при загрузке и скачивании больших файлов.
- **Интерактивный режим**: запуск без аргументов открывает REPL для последовательного выполнения команд.
- **Индикатор прогресса**: наглядная полоса прогресса при передаче файлов.

## 📦 Установка

### Загрузка бинарника
Скачайте последний бинарник со страницы Releases (скоро).

### Сборка из исходников

Требуется Go 1.24 или новее.

```bash
git clone https://github.com/yourusername/tbc.git
cd tbc
go build -o tbc.exe ./cmd/tbc
```

## 🚀 Использование

### 🔑 Аутентификация

Нужен cookie TeraBox (`ndus`). Получите его через инструменты разработчика в браузере и задайте в переменной окружения `TERABOX_COOKIE` либо сохраните в файл и укажите его через `-c`. Если ни `-c`, ни `TERABOX_COOKIE` не заданы, автоматически читается файл `./cookie`.

Самый надёжный способ — скопировать **весь заголовок `Cookie`** из `DevTools → Network` (любой запрос к `/api/...`), так как некоторым аккаунтам, помимо `ndus`, нужны cookie `ndut_fmt`, `csrfToken` и `TSID`.

Если ваш аккаунт работает через другой фронтенд TeraBox, укажите его API-хост с помощью `--base-url` (или `TERABOX_BASE_URL`), например `--base-url https://dm.terabox.com`.

```bash
# Задать через переменную окружения (PowerShell)
$env:TERABOX_COOKIE = "ndus=ВАШ_COOKIE"

# Или загрузить из файла
./tbc.exe -c cookie.txt ls
```

### 🌐 API-хост (`--base-url`)

Некоторые аккаунты обслуживаются другим фронтендом TeraBox, и `www.terabox.com` не принимает выданные им cookie. В этом случае укажите хост из адресной строки браузера. `--base-url` (алиас `-u`) — глобальный флаг, он действует для всех команд.

```powershell
# Полный адрес
.\tbc.exe --base-url https://dm.terabox.com ls

# Короткий алиас; схема https:// добавляется автоматически
.\tbc.exe -u dm.terabox.com ls

# Можно указывать и после имени команды
.\tbc.exe ls -u dm.terabox.com

# Вместе с файлом cookie
.\tbc.exe -c cookie -u https://dm.terabox.com get video.mp4
```

В интерактивном режиме:

```text
tbc> -u https://dm.terabox.com ls
```

Приоритет значений: `--base-url`/`-u` → переменная `TERABOX_BASE_URL` (можно задать в `.env`) → `https://www.terabox.com`.

### 💻 Команды

```text
NAME:
   tbc - TeraBox CLI client
            Run without arguments to enter interactive mode.

USAGE:
   tbc [global options] [command [command options]]

AUTHOR:
   SHA-5010

COMMANDS:
    ls       Вывести список файлов в удаленном каталоге
    mv       Переместить удаленные файлы или каталоги
    cp       Скопировать удаленные файлы или каталоги
    rm       Удалить удаленные файлы или каталоги
    mkdir    Создать удаленный каталог
    find     Поиск файлов в удаленном каталоге
    info     Показать информацию о пользователе
    put      Загрузить файлы в TeraBox
    get      Скачать файл из TeraBox
    df       Показать использование дискового пространства TeraBox
    help, h  Показать список команд или справку по конкретной команде

GLOBAL OPTIONS:
   --cookie-file string, -c string  TeraBox cookie file
       # if not specified, use TERABOX_COOKIE environment variable
   --base-url string, -u string     TeraBox API base URL
       # if not specified, use TERABOX_BASE_URL environment variable
   --help, -h  show help
```

### 💡 Примеры

#### Список файлов (`ls`)
Поддерживаются стандартные флаги `ls`: `-l` (подробный формат), `-h` (удобочитаемые размеры), `-r` (обратный порядок), `-t` (сортировка по времени) и `-S` (сортировка по размеру).

```bash
tbc ls                  # Показать текущий каталог
tbc ls /path/to/dir     # Показать указанный каталог
tbc ls -l               # Подробный список (размер, дата и т.д.)
tbc ls -lh              # Размеры в удобочитаемом виде
```

#### Скачивание (`get`)
```bash
tbc get remote_file.mp4
tbc get -d ./downloads remote_file.mp4
```

#### Загрузка (`put`)
```bash
tbc put local_file.txt /remote/dir
```

#### Интерактивный режим
Запустите без аргументов, чтобы войти в интерактивный режим.

```bash
$ ./tbc.exe
tbc> ls -l
... (список файлов) ...
tbc> get video.mp4
... (начинается скачивание) ...
tbc> exit
```

## 📄 Лицензия

MIT License
