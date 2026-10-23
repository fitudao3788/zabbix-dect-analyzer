# zabbix-dect-analyzer

## 概要
`zabbix-dect-analyzer` は、DECT（1.9GHz帯）によるCS干渉の兆候を Mirakurun のストリームから TS パケット情報として取得し、Zabbix へ定期送信するツールです。

データフローは次の通りです。

`Mirakurun → HTTPストリーム → 一時TSファイル → tspacketchk → zabbix_sender → Zabbix`

## 前提条件
- Docker / Docker Compose が利用できること。
- Mirakurun に到達可能で、対象サービスID（`mirakurun.target_service`）が存在すること。
- Zabbix Server 側で trapper 等、`zabbix_sender` から受信できる設定があること。
- コンテナ内で `tspacketchk` と `zabbix_sender` を利用します。`Dockerfile` では `tspacketchk Ver1.0.1` と `zabbix-sender`（`zabbix_sender` コマンド）を導入しています。

## 設定手順
1. 設定ファイルを作成します。

```bash
cp config.example.yml config.yml
```

2. `config.yml` を編集します（例）。

```yaml
log_level: info

mirakurun:
  host: host.docker.internal
  port: 40772
  target_service: 700333

streaming:
  duration_minutes: 1
  period_minutes: 10

zabbix:
  host: host.docker.internal
  port: 10051
  server_name: Zabbix server
  item_key_prefix: dect
```

設定項目の意味:
- `log_level`: ログレベル（`debug` / `info` / `warn` / `error`）。
- `mirakurun.host`: Mirakurun ホスト名またはIP。
- `mirakurun.port`: Mirakurun ポート番号。
- `mirakurun.target_service`: 取得対象のサービスID。
- `streaming.duration_minutes`: 1回の計測でストリームを取得する時間（分）。
- `streaming.period_minutes`: 次回計測までの間隔（分）。
- `zabbix.host`: `zabbix_sender` の送信先 Zabbix ホスト。
- `zabbix.port`: `zabbix_sender` の送信先 Zabbix ポート。
- `zabbix.server_name`: Zabbix で受信対象となるホスト名。
- `zabbix.item_key_prefix`: アイテムキー接頭辞。

3. Docker Compose では `./config.yml` をコンテナ内 `/app/config.yml` に read-only でマウントします。

## 起動・停止・確認
```bash
docker compose build
docker compose up -d
docker compose logs -f
docker compose ps
docker compose down
```

- `config.yml` が存在しない場合、`docker-compose.yml` の bind 設定（`create_host_path: false`）により起動できません。
- さらにアプリ起動後も `config.yml` がない場合は起動処理を終了します。

## Zabbix側の設定
- ホスト名は `zabbix.server_name` と一致させてください。
- アイテムキーは次の4つを作成してください。
  - `<item_key_prefix>.packets`（TSパケット数）
  - `<item_key_prefix>.drop`（ドロップ数）
  - `<item_key_prefix>.error`（エラー数）
  - `<item_key_prefix>.scrambling`（スクランブル数）
- `zabbix_sender` の送信先は `zabbix.host:zabbix.port` です。

## 動作仕様
- 起動直後に1回計測を実行し、その後 `period_minutes` 間隔で繰り返します。
- 1回の計測では `duration_minutes` 分間 Mirakurun ストリームを取得します。
- Mirakurun へのHTTPリクエストには `X-Mirakurun-Priority: -1` を付与します。
- `SIGINT`（Ctrl+C）を受けると終了します。

## トラブルシューティング
- `config.yml` がない / 読めない / YAML不正
  - `cp config.example.yml config.yml` を再実行し、YAMLのインデントとキー名を確認してください。
- `tspacketchk` または `zabbix_sender` が見つからない
  - `docker compose build --no-cache` で再ビルドしてください。
- Mirakurun への接続失敗、対象サービスID不正
  - `mirakurun.host` / `mirakurun.port` / `mirakurun.target_service` を見直してください。
- TS packet info の解析失敗
  - 取得ストリームに問題がないか、対象サービスIDが正しいかを確認してください。
- Zabbix送信失敗
  - `zabbix.host` / `zabbix.port`、および Zabbix trapper 設定を確認してください。
- `log_level: debug` で確認できる内容
  - 一時TSファイルパス
  - Mirakurun リクエストURL、レスポンスステータス・ヘッダー
  - `tspacketchk` 出力（解析前ログ）
  - `zabbix_sender` 入力データとコマンド出力

## 注意事項
- `duration_minutes` と `period_minutes` は正の値を設定してください。
- `docker-compose.yml` は `host.docker.internal:host-gateway` を使って、コンテナからホスト上の Mirakurun/Zabbix に接続する構成です。Linux環境や Mirakurun/Zabbix が別ホストにある場合は、`mirakurun.host` / `zabbix.host` と `extra_hosts` の設定を環境に合わせて見直してください。
- 実装上は、ストリーム取得がタイムアウトで終了した場合でも、その時点までに取得できた内容で後続処理（解析・送信）へ進む前提です。

## ライセンス
Apache License 2.0（`LICENSE` を参照）。
