# TeraBox CLI (tbc)

![tbc_logo](https://repository-images.githubusercontent.com/963345828/78ea40a2-977c-4de4-b24d-74d9f3cef4c6)

**English** · [Русский](README_ru.md) · [日本語](README_ja.md)

A simple and powerful command-line client for TeraBox, written in Go.
It supports `ls`-like file listing, memory-efficient split upload/download, and an interactive mode (REPL).

## ✨ Features

- **Unix-like Experience**: Familiar commands like `ls`, `cp`, `mv`, `rm`.
- **Rich Display**: `ls` command supports ANSI colors and grid display, automatically adjusting to terminal width.
- **Memory Efficient**: Implements streaming processing to minimize memory usage even during large file uploads/downloads.
- **Interactive Mode**: Enter REPL mode by running without arguments to execute commands continuously.
- **Progress Bar**: Visual progress bar for transfers.

## 📦 Installation

### Download Binary
Download the latest binary from the Releases page (Coming soon).

### Build from Source

Requires Go 1.24 or higher.

```bash
git clone https://github.com/yourusername/tbc.git
cd tbc
go build -o tbc.exe ./cmd/tbc
```

## 🚀 Usage

### 🔑 Authentication

You need a TeraBox cookie (`ndus`). Get it via browser developer tools and set it in the `TERABOX_COOKIE` environment variable, or save it to a file and use the `-c` option.

The most reliable way is to copy the **whole `Cookie` request header** from `DevTools → Network` (any `/api/...` request), because some accounts also need `ndut_fmt`, `csrfToken` and `TSID` cookies in addition to `ndus`.

If your account uses another TeraBox frontend, point the client at its API host with `--base-url` (or `TERABOX_BASE_URL`), e.g. `--base-url https://dm.terabox.com`.

```bash
# Set via environment variable (PowerShell)
$env:TERABOX_COOKIE = "ndus=Yb1zciYteHuiRbjO_aw3cCbi540KkqudGmMc5gd3"

# Or load from file
./tbc.exe -c cookie.txt ls
```

### 💻 Commands

```text
NAME:
   tbc - TeraBox CLI client
            Run without arguments to enter interactive mode.

USAGE:
   tbc [global options] [command [command options]]

AUTHOR:
   SHA-5010

COMMANDS:
   ls       List files in a remote directory
   mv       Move remote files or directories
   cp       Copy remote files or directories
   rm       Remove remote files or directories
   mkdir    Make remote directory
   find     Search for files in a remote directory
   info     Show user information
   put      Upload files to TeraBox
   get      Download file from TeraBox
   df       Display disk usage of TeraBox storage
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --cookie-file string, -c string  TeraBox cookie file
       # if not specified, use TERABOX_COOKIE environment variable
   --base-url string, -u string     TeraBox API base URL
       # if not specified, use TERABOX_BASE_URL environment variable
   --help, -h  show help
```

### 💡 Examples

#### List Files (`ls`)
Supports standard `ls` flags like `-l` (long format), `-h` (human-readable), `-r` (reverse), `-t` (sort by time), and `-S` (sort by size).

```bash
tbc ls                  # List current directory
tbc ls /path/to/dir     # List specific directory
tbc ls -l               # Long listing (size, date, etc.)
tbc ls -lh              # Human-readable sizes
```

#### Download (`get`)
```bash
tbc get remote_file.mp4
tbc get -d ./downloads remote_file.mp4
```

#### Upload (`put`)
```bash
tbc put local_file.txt /remote/dir
```

#### Interactive Mode
Run without arguments to enter interactive mode.

```bash
$ ./tbc.exe
tbc> ls -l
... (file list) ...
tbc> get video.mp4
... (download starts) ...
tbc> exit
```

## 📄 License

MIT License
