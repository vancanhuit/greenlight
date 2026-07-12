package main

import (
	"context"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vancanhuit/greenlight/internal/data"
	"github.com/vancanhuit/greenlight/internal/mailer"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	cors struct {
		trustedOrigins []string
	}
	tls struct {
		certFile     string
		keyFile      string
		clientCAFile string
		trustProxy   bool
	}
}

type application struct {
	config      config
	logger      *slog.Logger
	movies      MovieStore
	users       UserStore
	tokens      TokenStore
	permissions PermissionStore
	mailer      Emailer
	wg          sync.WaitGroup
	// shutdownCtx is cancelled when the server begins shutting down, signalling
	// background maintenance goroutines (e.g. the rate-limiter cleanup) to stop.
	shutdownCtx context.Context
}

func validateTLSConfig(certFile, keyFile, clientCAFile string) error {
	if (certFile == "") != (keyFile == "") {
		return errors.New("both -tls-cert-file and -tls-key-file must be set together")
	}
	if clientCAFile != "" && certFile == "" {
		return errors.New("-tls-client-ca-file requires -tls-cert-file and -tls-key-file")
	}
	return nil
}

func openDB(cfg config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = int32(cfg.db.maxOpenConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConnIdleTime = duration

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 8000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production")

	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("GREENLIGHT_DB_DSN"), "PostgreSQL DSN")

	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
	flag.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")

	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

	flag.StringVar(&cfg.smtp.host, "smtp-host", os.Getenv("GREENLIGHT_SMTP_HOST"), "SMTP host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "SMTP port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", os.Getenv("GREENLIGHT_SMTP_USERNAME"), "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", os.Getenv("GREENLIGHT_SMTP_PASSWORD"), "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender", os.Getenv("GREENLIGHT_SMTP_SENDER"), "SMTP sender")

	flag.Func("cors-trusted-origins", "Trusted CORS origins (space separated)", func(s string) error {
		cfg.cors.trustedOrigins = strings.Fields(s)
		return nil
	})

	flag.StringVar(&cfg.tls.certFile, "tls-cert-file", "", "TLS certificate file (enables direct TLS)")
	flag.StringVar(&cfg.tls.keyFile, "tls-key-file", "", "TLS private key file")
	flag.StringVar(&cfg.tls.clientCAFile, "tls-client-ca-file", "", "CA file to verify client certificates (enables mTLS)")
	flag.BoolVar(&cfg.tls.trustProxy, "trust-proxy", false, "Trust X-Forwarded-For/X-Real-IP headers for client IP (only enable behind a trusted proxy)")

	displayVersion := flag.Bool("version", false, "Display version and exit")

	flag.Parse()

	if *displayVersion {
		fmt.Printf("Version: \t%s\n", version)
		os.Exit(0)
	}

	if err := validateTLSConfig(cfg.tls.certFile, cfg.tls.keyFile, cfg.tls.clientCAFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	db, err := openDB(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("database connection pool established")

	expvar.NewString("version").Set(version)
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stat()
	}))
	expvar.Publish("timestamp", expvar.Func(func() any {
		return time.Now().Unix()
	}))

	models := data.NewModels(db)
	app := application{
		config:      cfg,
		logger:      logger,
		movies:      models.Movies,
		users:       models.Users,
		tokens:      models.Tokens,
		permissions: models.Permissions,
		mailer: mailer.New(
			cfg.smtp.host,
			cfg.smtp.port,
			cfg.smtp.username,
			cfg.smtp.password,
			cfg.smtp.sender,
		),
	}

	if err := app.migrateDB(db); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.Info("database migrations applied")

	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
