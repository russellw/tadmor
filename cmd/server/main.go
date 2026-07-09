// Command server runs the tadmor backend: it connects to Postgres, applies any
// pending schema migrations, and serves the HTTP API until interrupted.
//
// With -adduser it instead creates (or resets the password of) a login and
// exits; use this to bootstrap the first account. The user is an administrator
// unless -admin=false is given, which provisions an ordinary login (e.g. the
// demo's guest account):
//
//	echo 'the-password' | server -adduser -email you@example.com -name 'Your Name'
//	echo 'the-password' | server -adduser -admin=false -email guest@example.com -name 'Guest'
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tadmor/db/migrations"
	"tadmor/internal/auth"
	"tadmor/internal/config"
	"tadmor/internal/db"
	"tadmor/internal/httpapi"
	"tadmor/internal/mailer"
	"tadmor/web"
)

func main() {
	adduser := flag.Bool("adduser", false, "create or update a login user (password read from stdin), then exit")
	email := flag.String("email", "", "email of the user to add (with -adduser)")
	name := flag.String("name", "", "full name of the user to add (with -adduser)")
	admin := flag.Bool("admin", true, "make the added user an administrator (with -adduser); -admin=false for an ordinary login")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if *adduser {
		if err := runAddUser(*email, *name, *admin); err != nil {
			logger.Error("adduser failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if err := run(logger); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// runAddUser reads a password from the first line of stdin and upserts the
// user, applying pending migrations first so it works on a fresh database.
func runAddUser(email, name string, isAdmin bool) error {
	if email == "" || name == "" {
		return errors.New("-adduser requires -email and -name")
	}
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && password == "" {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Apply(ctx, pool, migrations.FS); err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	id, err := auth.UpsertUser(ctx, pool, email, name, hash, isAdmin)
	if err != nil {
		return err
	}
	role := "admin"
	if !isAdmin {
		role = "user"
	}
	fmt.Printf("%s %d (%s) ready\n", role, id, email)
	return nil
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	applied, err := db.Apply(ctx, pool, migrations.FS)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		logger.Info("applied migrations", "count", len(applied), "versions", applied)
	} else {
		logger.Info("database schema up to date")
	}

	mail := mailer.New(mailer.Config{
		Addr:     cfg.SMTPAddr,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPass,
		From:     cfg.MailFrom,
	}, logger)
	if cfg.SMTPAddr == "" {
		logger.Info("email sending disabled (SMTP_ADDR unset)")
	} else {
		logger.Info("email sending enabled", "smtp_addr", cfg.SMTPAddr)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewServer(pool, logger, httpapi.WithMailer(mail)).Handler(web.DistFS()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
