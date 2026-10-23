package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type MirakurunConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	TargetService string `yaml:"target_service"`
}

type StreamingConfig struct {
	DurationMinutes int `yaml:"duration_minutes"`
	PeriodMinutes   int `yaml:"period_minutes"`
}

type ZabbixConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	ServerName    string `yaml:"server_name"`
	ItemKeyPrefix string `yaml:"item_key_prefix"`
}

type Config struct {
	LogLevel  string          `yaml:"log_level"`
	Mirakurun MirakurunConfig `yaml:"mirakurun"`
	Streaming StreamingConfig `yaml:"streaming"`
	Zabbix    ZabbixConfig    `yaml:"zabbix"`
}

func NewConfig() *Config {
	return &Config{
		LogLevel: "info",
		Mirakurun: struct {
			Host          string `yaml:"host"`
			Port          int    `yaml:"port"`
			TargetService string `yaml:"target_service"`
		}{
			Host:          "127.0.0.1",
			Port:          40772,
			TargetService: "700333",
		},
		Streaming: StreamingConfig{
			DurationMinutes: 1,
			PeriodMinutes:   10,
		},
		Zabbix: ZabbixConfig{
			Host:          "127.0.0.1",
			Port:          10051,
			ServerName:    "Zabbix server",
			ItemKeyPrefix: "dect",
		},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// ログのデフォルト設定
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// 必須コマンドのチェック
	for _, program := range []string{"tspacketchk", "zabbix_sender"} {
		if _, err := exec.LookPath(program); err != nil {
			slog.Error("failed to find program", "program", program, "err", err)
			return
		}
	}

	// config.ymlのチェック
	if c_raw, err := os.Stat("config.yml"); err != nil || c_raw.IsDir() {
		slog.Error("failed to find config.yml", "err", err)
		return
	}

	// config.ymlを開く
	c_data, err := os.ReadFile("config.yml")
	if err != nil {
		slog.Error("failed to read config.yml", "err", err)
		return
	}

	// config.ymlをデコード
	config := NewConfig()
	if err := yaml.Unmarshal(c_data, &config); err != nil {
		slog.Error("failed to decode config.yml", "err", err)
		return
	}

	// ログレベルの設定
	logLevel := strings.ToLower(config.LogLevel)
	switch logLevel {
	case "debug":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	case "info":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	case "warn":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	case "error":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	}

	// 定期的に実行する
	tiker := time.NewTicker(time.Duration(config.Streaming.PeriodMinutes) * time.Minute)
	defer tiker.Stop()

	get_info(config)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tiker.C:
			get_info(config)
		}
	}
}

func get_info(config *Config) {
	// 一時ファイルを作成
	tmp_file, err := os.CreateTemp("", "stream_*.ts")
	if err != nil {
		return
	}
	defer os.Remove(tmp_file.Name())
	slog.Debug("creating temporary file", "path", tmp_file.Name())

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Streaming.DurationMinutes)*time.Minute)
	defer cancel()

	// ストリームを取得
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/api/services/%s/stream", config.Mirakurun.Host, config.Mirakurun.Port, config.Mirakurun.TargetService), nil)
	req.Header.Add("X-Mirakurun-Priority", "-1")
	slog.Debug("requesting stream", "url", req.URL.String())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("failed to get stream", "err", err)
		return
	}
	defer res.Body.Close()
	slog.Debug("response status", "status", res.Status)
	for key, values := range res.Header {
		for _, value := range values {
			slog.Debug("response header", "key", key, "value", value)
		}
	}

	// レスポンスを一時ファイルに書き込む
	_, err = io.Copy(tmp_file, res.Body)
	if !errors.Is(err, context.DeadlineExceeded) {
		slog.Error("failed to write stream", "err", err)
		return
	}

	chk_cmd := exec.Command("tspacketchk", "-l", "0", tmp_file.Name())
	info_out, err := chk_cmd.Output()
	if err != nil {
		slog.Error("failed to get ts packet info", "err", err)
		return
	}

	// 出力結果の各値を抽出
	packets_val := int64(-1)
	drop_val := int64(-1)
	error_val := int64(-1)
	scrambling_val := int64(-1)
	sep_found_before := false
	sep_count := 0
	info_scan := bufio.NewScanner(bytes.NewReader(info_out))
	for info_scan.Scan() {
		slog.Debug(info_scan.Text())

		// 区切り文字を検出
		if strings.HasPrefix(info_scan.Text(), "------") {
			sep_count++
			sep_found_before = true

			continue
		}

		// 区切り文字後の各値を抽出
		if sep_found_before && sep_count == 2 {
			values := strings.Fields(info_scan.Text())
			if len(values) == 4 {
				packets_val, _ = strconv.ParseInt(values[0], 10, 64)
				drop_val, _ = strconv.ParseInt(values[1], 10, 64)
				error_val, _ = strconv.ParseInt(values[2], 10, 64)
				scrambling_val, _ = strconv.ParseInt(values[3], 10, 64)
			}

			break
		}

		sep_found_before = false
	}

	if packets_val == -1 || drop_val == -1 || error_val == -1 || scrambling_val == -1 {
		slog.Error("failed to parse ts packet info")
		return
	}

	slog.Info("parsed ts packet info", "packets", packets_val, "drop", drop_val, "error", error_val, "scrambling", scrambling_val)

	// Zabbixに送信するデータを作成
	var snd_data bytes.Buffer
	fmt.Fprintf(&snd_data, "\"%s\" %s.packets %d\n", config.Zabbix.ServerName, config.Zabbix.ItemKeyPrefix, packets_val)
	fmt.Fprintf(&snd_data, "\"%s\" %s.drop %d\n", config.Zabbix.ServerName, config.Zabbix.ItemKeyPrefix, drop_val)
	fmt.Fprintf(&snd_data, "\"%s\" %s.error %d\n", config.Zabbix.ServerName, config.Zabbix.ItemKeyPrefix, error_val)
	fmt.Fprintf(&snd_data, "\"%s\" %s.scrambling %d\n", config.Zabbix.ServerName, config.Zabbix.ItemKeyPrefix, scrambling_val)
	slog.Debug("zabbix_sender input", "input", snd_data.String())

	// Zabbixサーバに送信
	snd_cmd := exec.Command("zabbix_sender", "-z", config.Zabbix.Host, "-p", fmt.Sprintf("%d", config.Zabbix.Port), "-i", "-")
	snd_cmd.Stdin = &snd_data
	snd_out, err := snd_cmd.CombinedOutput()
	if err != nil {
		slog.Error("failed to send data to zabbix", "err", err)
	}
	slog.Debug("zabbix_sender output", "output", string(snd_out))
}
