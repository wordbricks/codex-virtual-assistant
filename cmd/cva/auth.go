package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	authpkg "github.com/siisee11/CodexVirtualAssistant/internal/auth"
	"github.com/siisee11/CodexVirtualAssistant/internal/config"
	"golang.org/x/term"
)

type authHashOutput struct {
	PasswordHash string `json:"password_hash"`
}

type authRegisterOutput struct {
	UserID      string `json:"user_id"`
	ConfigFile  string `json:"config_file"`
	AuthEnabled bool   `json:"auth_enabled"`
}

func cmdAuth(args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cva auth <hash-password|register>")
	}
	switch args[0] {
	case "hash-password":
		if len(args) != 1 {
			return fmt.Errorf("usage: cva auth hash-password")
		}
		password, err := readPasswordForHash()
		if err != nil {
			return err
		}
		passwordHash, err := authpkg.HashPassword(password)
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(authHashOutput{PasswordHash: passwordHash})
		}
		fmt.Println(passwordHash)
		return nil
	case "register":
		if len(args) != 1 {
			return fmt.Errorf("usage: cva auth register")
		}
		output, err := registerAuth(jsonMode)
		if err != nil {
			return err
		}
		if jsonMode {
			return printJSON(output)
		}
		fmt.Fprintf(os.Stdout, "Authentication enabled for %s\nConfig File: %s\n", output.UserID, output.ConfigFile)
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand: %s", args[0])
	}
}

func registerAuth(jsonMode bool) (authRegisterOutput, error) {
	reader := bufio.NewReader(os.Stdin)
	userID, err := readAuthUserID(reader, os.Stderr)
	if err != nil {
		return authRegisterOutput{}, err
	}
	password, err := readRegisterPassword(reader, os.Stderr, "Password: ")
	if err != nil {
		return authRegisterOutput{}, err
	}
	confirmation, err := readRegisterPassword(reader, os.Stderr, "Confirm password: ")
	if err != nil {
		return authRegisterOutput{}, err
	}
	if password != confirmation {
		return authRegisterOutput{}, fmt.Errorf("password confirmation did not match")
	}

	passwordHash, err := authpkg.HashPassword(password)
	if err != nil {
		return authRegisterOutput{}, err
	}
	configDir, err := config.ResolveConfigDir()
	if err != nil {
		return authRegisterOutput{}, fmt.Errorf("resolve config directory: %w", err)
	}
	if err := config.WriteAuthConfig(configDir, userID, passwordHash); err != nil {
		return authRegisterOutput{}, fmt.Errorf("write auth config: %w", err)
	}
	if !jsonMode {
		fmt.Fprintln(os.Stderr, "Password hash saved. Restart the CVA server to apply authentication.")
	}
	return authRegisterOutput{
		UserID:      userID,
		ConfigFile:  config.ConfigFilePath(configDir),
		AuthEnabled: true,
	}, nil
}

func readAuthUserID(reader *bufio.Reader, stderr io.Writer) (string, error) {
	fmt.Fprint(stderr, "User ID [admin]: ")
	raw, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read user id: %w", err)
	}
	userID := strings.TrimSpace(raw)
	if userID == "" {
		userID = "admin"
	}
	if err := authpkg.ValidateUserID(userID); err != nil {
		return "", err
	}
	return userID, nil
}

func readRegisterPassword(reader *bufio.Reader, stderr io.Writer, prompt string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, prompt)
		data, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(data), nil
	}
	fmt.Fprint(stderr, prompt)
	raw, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(raw, "\r\n"), nil
}

func readPasswordForHash() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Password: ")
		first, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}

		fmt.Fprint(os.Stderr, "Confirm password: ")
		second, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password confirmation: %w", err)
		}
		if string(first) != string(second) {
			return "", fmt.Errorf("password confirmation did not match")
		}
		return string(first), nil
	}

	data, err := io.ReadAll(io.LimitReader(os.Stdin, 2048))
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}
