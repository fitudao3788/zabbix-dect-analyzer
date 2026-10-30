# zabbix-dect-analyzer

DECT(1.9GHz帯)によるCS受信への干渉を、Zabbixに記録するツールです。

Mirakurun / mirakc から対象サービスのストリームを定期的に取得し、TSパケットの異常数を計測して Zabbix に送信します。干渉の発生時刻や頻度を、グラフで確認できるようになります。

## 仕組み

1. Mirakurun / mirakc の API から、指定サービスのストリームを一定時間取得し、一時ファイルに保存します。
   - 優先度は最低(`X-Mirakurun-Priority: -1`)にしてあり、通常の録画や視聴を妨げません。
2. [tspacketchk](https://github.com/kaikoma-soft/tspacketchk) でTSパケットを検査し、次の4つを取得します。
   - `packets`: 総パケット数
   - `drop`: ドロップ数
   - `error`: エラー数
   - `scrambling`: スクランブル数
3. `zabbix_sender` で Zabbix に送信します。
4. 1〜3を指定した間隔で繰り返します。

## 必要なもの

- Docker / Docker Compose
- Mirakurun または mirakc
- Zabbix サーバー

## 使い方

### 1. Zabbix にアイテムを作成する

対象ホストに、次のキーで **Zabbix トラッパー** のアイテム(数値・整数)を作成します。

| キー | 内容 |
|---|---|
| `dect.packets` | 総パケット数 |
| `dect.drop` | ドロップ数 |
| `dect.error` | エラー数 |
| `dect.scrambling` | スクランブル数 |

キーの先頭 `dect` は、設定の `item_key_prefix` で変更できます。ドロップ率などの割合は、Zabbix の計算アイテムで求められます。

### 2. 設定ファイルを作成する

```bash
cp config.example.yml config.yml
```

`config.yml` を環境に合わせて編集します。

```yaml
log_level: info            # debug / info / warn / error

mirakurun:
  host: host.docker.internal
  port: 40772
  target_service: 700333   # 計測するサービスID

streaming:
  duration_minutes: 1      # 1回の計測でストリームを取得する時間(分)
  period_minutes: 10       # 計測の間隔(分)

zabbix:
  host: host.docker.internal
  port: 10051
  server_name: Zabbix server  # Zabbix 上のホスト名
  item_key_prefix: dect
```

### 3. 起動する

```bash
docker compose up -d --build
```

ログは `docker compose logs -f` で確認できます。

## Docker を使わずに実行する

`tspacketchk` と `zabbix_sender` を PATH に通し、`config.yml` のあるディレクトリで実行します。

```bash
go build -o zabbix-dect-analyzer .
./zabbix-dect-analyzer
```

## ライセンス

Apache License 2.0
