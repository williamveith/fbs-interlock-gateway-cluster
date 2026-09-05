package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const (
	r2AccessKeySecret = "/run/secrets/r2_access_key_id"
	r2SecretKeySecret = "/run/secrets/r2_secret_access_key"
)

func main() {
	if err := loadSecret("R2_ACCESS_KEY_ID", r2AccessKeySecret); err != nil {
		fatal(err)
	}

	if err := loadSecret("R2_SECRET_ACCESS_KEY", r2SecretKeySecret); err != nil {
		fatal(err)
	}

	path, err := exec.LookPath("litestream")
	if err != nil {
		fatal(fmt.Errorf("find litestream: %w", err))
	}

	args := []string{"litestream"}

	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	} else {
		args = append(args, "replicate")
	}

	if err := syscall.Exec(path, args, os.Environ()); err != nil {
		fatal(fmt.Errorf("exec litestream: %w", err))
	}
}

func loadSecret(envName, path string) error {
	// Preserve normal docker-run development behavior where the
	// credential has already been supplied as an environment variable.
	if strings.TrimSpace(os.Getenv(envName)) != "" {
		return nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s from %s: %w", envName, path, err)
	}

	value := strings.TrimSpace(string(contents))
	if value == "" {
		return fmt.Errorf("%s is empty", path)
	}

	if err := os.Setenv(envName, value); err != nil {
		return fmt.Errorf("set %s: %w", envName, err)
	}

	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "swarm-entrypoint:", err)
	os.Exit(1)
}
