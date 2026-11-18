package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/api"
	"ecs-cdt-panel/internal/auth"
	"ecs-cdt-panel/internal/config"
	"ecs-cdt-panel/internal/crypto"
	"ecs-cdt-panel/internal/db"
	"ecs-cdt-panel/internal/notify"
	"ecs-cdt-panel/internal/scheduler"
	"ecs-cdt-panel/web"
)

func main() {
	// Keep Go runtime heap compact in low-memory containers unless overridden via GOMEMLIMIT.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(48 << 20)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "err", err)
		os.Exit(1)
	}

	if cfg.GeneratedPassword {
		slog.Warn("PANEL_PASSWORD 未配置，本次启动已生成临时登录密码（建议在环境变量中配置 PANEL_PASSWORD）",
			"username", cfg.PanelUsername,
			"password", cfg.PanelPassword,
		)
	}

	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(cfg.DataDir, cipher)
	if err != nil {
		slog.Error("open database failed", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		slog.Error("load embedded static assets failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	aliClient := aliyun.NewClient()
	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	authMgr := auth.NewManager(cfg)
	handler := api.NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	go engine.Start(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("ECS CDT 管理面板已启动", "port", cfg.Port, "data_dir", cfg.DataDir, "username", cfg.PanelUsername)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("正在关闭服务...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	engine.Wait()
}
