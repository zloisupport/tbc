# TeraBox CLI (tbc)

![tbc_logo](https://repository-images.githubusercontent.com/963345828/78ea40a2-977c-4de4-b24d-74d9f3cef4c6)

[English](README.md) · [Русский](README_ru.md) · **日本語**

Go言語で実装された、シンプルで強力なTeraBoxコマンドラインクライアントです。
`ls` コマンドのようなファイル一覧表示、メモリ効率の良い分割アップロード・ダウンロード、そしてインタラクティブモード（REPL）をサポートしています。

## ✨ 特徴

- **Unixライクな操作感**: `ls`, `cp`, `mv`, `rm` などの馴染みのあるコマンド体系。
- **リッチな表示**: `ls` コマンドはANSIカラーとグリッド表示に対応し、ターミナル幅に合わせて自動調整されます。
- **メモリ効率**: 大容量ファイルのアップロード・ダウンロード時もメモリ使用量を最小限に抑えるストリーミング処理を実装。
- **インタラクティブモード**: 引数なしで起動するとREPLモードに入り、連続してコマンドを実行可能。
- **プログレスバー**: 転送状況を視覚的に確認できるプログレスバー。

## 📦 インストール

### バイナリのダウンロード
Releases ページから最新のバイナリをダウンロードしてください（準備中）。

### ソースコードからビルド

Go 1.24以上が必要です。

```bash
git clone https://github.com/yourusername/tbc.git
cd tbc
go build -o tbc.exe ./cmd/tbc
```

## 🚀 使い方

### 🔑 認証

TeraBoxのクッキー（`ndus`）が必要です。ブラウザの開発者ツールなどで取得し、環境変数 `TERABOX_COOKIE` に設定するか、ファイルに保存して `-c` オプションで指定してください。

```bash
# 環境変数で設定 (PowerShell)
$env:TERABOX_COOKIE = "ndus=YOUR_COOKIE_VALUE; ..."

# またはファイルから読み込み
./tbc.exe -c cookie.txt ls
```

### 💻 コマンド一覧

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
   --help, -h  show help
```

### 💡 コマンド例

#### ファイル一覧 (`ls`)
標準的な `ls` コマンドと同様に、`-l`（詳細表示）、`-h`（人間が読みやすいサイズ）、`-r`（逆順）、`-t`（時間順）、`-S`（サイズ順）などのフラグをサポートしています。

```bash
tbc ls                  # カレントディレクトリを表示
tbc ls /path/to/dir     # 指定ディレクトリを表示
tbc ls -l               # 詳細表示（サイズ、日付など）
tbc ls -lh              # 人間が読みやすいサイズ表記
```

#### ダウンロード (`get`)
```bash
tbc get remote_file.mp4
tbc get -d ./downloads remote_file.mp4
```

#### アップロード (`put`)
```bash
tbc put local_file.txt /remote/dir
```

#### インタラクティブモード
引数なしで起動すると、対話モードに入ります。

```bash
$ ./tbc.exe
tbc> ls -l
... (ファイル一覧) ...
tbc> get video.mp4
... (ダウンロード開始) ...
tbc> exit
```

## 📄 ライセンス

MIT License
