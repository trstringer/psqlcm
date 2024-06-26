package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

type connection struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	SSLMode  string `json:"sslmode"`
}

func (c connection) String() string {
	connectionString := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s", c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode)
	return connectionString
}

const (
	flagCacheDir   string = "cache-dir"
	flagNotCurrent string = "not-current"

	defaultHostname string = "localhost"
	defaultPort     int    = 5432
	defaultUser     string = "postgres"
	defaultDatabase string = "postgres"
	defaultSSLMode  string = "require"

	keyEnvVar             string = "PSQLCM_KEY"
	currentConnectionName string = "current"
)

func deleteConnection(cCtx *cli.Context) error {
	if cCtx.Args().Len() == 0 {
		return fmt.Errorf("you must pass the connection name to show")
	}
	connectionName := cCtx.Args().Get(0)
	fullPath := filepath.Join(cCtx.String(flagCacheDir), connectionName)
	_, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("connection not found")
	}

	connectionIsCurrent, err := isConnectionCurrent(connectionName, cCtx.String(flagCacheDir))
	if err != nil {
		return fmt.Errorf("error checking for current connection: %w", err)
	}
	if connectionIsCurrent {
		removeCurrent(cCtx.String(flagCacheDir))
	}

	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("error deleting connection file: %w", err)
	}

	fmt.Printf("Connection %q deleted\n", connectionName)

	return nil
}

func show(cCtx *cli.Context) error {
	var connectionName string
	if cCtx.Args().Len() == 0 {
		connectionName = currentConnectionName
	} else {
		connectionName = cCtx.Args().Get(0)
	}
	fullPath := filepath.Join(cCtx.String(flagCacheDir), connectionName)
	output, err := os.ReadFile(fullPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("connection does not exist, run `psqlcm ls`")
		} else {
			return fmt.Errorf("error reading connection file: %w", err)
		}
	}

	cachedConnection := connection{}
	if err := json.Unmarshal(output, &cachedConnection); err != nil {
		return fmt.Errorf("error unmarshalling file: %w", err)
	}
	cachedConnection.Password, err = decryptPassword(cachedConnection.Password)
	if err != nil {
		return fmt.Errorf("error decrypting password: %w", err)
	}
	fmt.Printf(cachedConnection.String())

	return nil
}

func removeCurrent(dir string) error {
	return os.Remove(filepath.Join(dir, currentConnectionName))
}

func isConnectionCurrent(connectionName, dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, currentConnectionName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("error checking for current file: %w", err)
	}
	dst, err := os.Readlink(filepath.Join(dir, currentConnectionName))
	if err != nil {
		return false, fmt.Errorf("error reading current link: %w", err)
	}
	pathParts := strings.Split(dst, "/")
	currentConnection := pathParts[len(pathParts)-1]
	return currentConnection == connectionName, nil
}

func list(cCtx *cli.Context) error {
	dirs, err := os.ReadDir(cCtx.String(flagCacheDir))
	if err != nil {
		return fmt.Errorf("error reading cache dir: %w", err)
	}

	var currentConnection string
	for _, dir := range dirs {
		if dir.Name() == currentConnectionName {
			dst, err := os.Readlink(filepath.Join(cCtx.String(flagCacheDir), currentConnectionName))
			if err != nil {
				return fmt.Errorf("error reading current link: %w", err)
			}
			pathParts := strings.Split(dst, "/")
			currentConnection = pathParts[len(pathParts)-1]
		}
	}

	for _, dir := range dirs {
		printableConnection := dir.Name()
		if printableConnection == currentConnectionName {
			continue
		}
		if printableConnection == currentConnection {
			printableConnection = fmt.Sprintf("*%s", printableConnection)
		}
		fmt.Println(printableConnection)
	}
	return nil
}

func generateConnectionName() string {
	now := time.Now()
	return fmt.Sprintf("pg%v", now.UnixMilli())
}

func setCurrent(connectionName, path string) error {
	src := filepath.Join(path, connectionName)
	dst := filepath.Join(path, currentConnectionName)

	if _, err := os.Stat(dst); err == nil {
		os.Remove(dst)
	}

	return os.Symlink(src, dst)
}

func newConnection(cCtx *cli.Context) error {
	newConnection := &connection{}

	fmt.Printf("🖥️ Hostname [%s]: ", defaultHostname)
	fmt.Scanln(&newConnection.Host)
	if newConnection.Host == "" {
		newConnection.Host = defaultHostname
	}

	fmt.Printf("🌐 Port [%d]: ", defaultPort)
	var portRaw string
	var err error
	fmt.Scanln(&portRaw)
	if portRaw == "" {
		newConnection.Port = defaultPort
	} else {
		newConnection.Port, err = strconv.Atoi(portRaw)
		if err != nil {
			return fmt.Errorf("error converting port to int: %w", err)
		}
	}

	fmt.Printf("📝 Database [%s]: ", defaultDatabase)
	fmt.Scanln(&newConnection.Database)
	if newConnection.Database == "" {
		newConnection.Database = defaultDatabase
	}

	fmt.Printf("🔨 User [%s]: ", defaultUser)
	fmt.Scanln(&newConnection.User)
	if newConnection.User == "" {
		newConnection.User = defaultUser
	}

	fmt.Printf("🔑 Password: ")
	inputPassword, err := term.ReadPassword(0)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("error getting password: %w", err)
	}
	newConnection.Password = string(inputPassword)

	fmt.Printf("🔒 SSL mode [%s]: ", defaultSSLMode)
	var sslMode string
	fmt.Scanln(&sslMode)
	if sslMode == "" {
		sslMode = defaultSSLMode
	}
	if !slices.Contains([]string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}, sslMode) {
		return fmt.Errorf("unsupported SSL mode")
	}
	newConnection.SSLMode = sslMode

	defaultConnectionName := generateConnectionName()
	fmt.Printf("\n📕 Connection name [%s]: ", defaultConnectionName)
	var connectionName string
	fmt.Scanln(&connectionName)
	if connectionName == "" {
		connectionName = defaultConnectionName
	}

	fmt.Printf("⚡ Test connection [Y/n]: ")
	var testConnectionAnswer string
	fmt.Scanln(&testConnectionAnswer)
	if testConnectionAnswer == "" || strings.ToLower(string(testConnectionAnswer[0])) == "y" {
		dbConnection, err := sql.Open("postgres", newConnection.String())
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			fmt.Printf("Save connection? [Y/n] ")
			var continueAnyway string
			fmt.Scanln(&continueAnyway)
			if continueAnyway != "" && strings.ToLower(string(continueAnyway[0])) != "y" {
				return fmt.Errorf("error opening connection: %w", err)
			}
		}
		if err := dbConnection.Ping(); err != nil {
			fmt.Printf("Error: %v\n", err)
			fmt.Printf("Save connection? [Y/n] ")
			var continueAnyway string
			fmt.Scanln(&continueAnyway)
			if continueAnyway != "" && strings.ToLower(string(continueAnyway[0])) != "y" {
				return fmt.Errorf("error pinging database: %w", err)
			}
		}
	}

	_, err = os.Stat(cCtx.String(flagCacheDir))
	if err != nil {
		err = os.Mkdir(cCtx.String(flagCacheDir), os.ModePerm)
		if err != nil {
			return fmt.Errorf("error making output cache dir: %w", err)
		}
	}

	newConnection.Password, err = encryptPassword(newConnection.Password)
	if err != nil {
		return fmt.Errorf("error encrypting password: %w", err)
	}
	connectionJSON, err := json.MarshalIndent(newConnection, "", "    ")
	if err != nil {
		return fmt.Errorf("error marshaling output: %w", err)
	}

	err = os.WriteFile(
		filepath.Join(cCtx.String(flagCacheDir), connectionName),
		connectionJSON,
		0644,
	)
	if err != nil {
		return fmt.Errorf("error writing connection to file: %w", err)
	}

	notCurrent := cCtx.Bool(flagNotCurrent)
	if !notCurrent {
		if err := setCurrent(connectionName, cCtx.String(flagCacheDir)); err != nil {
			return fmt.Errorf("error setting connection as current: %w", err)
		}
	}

	fmt.Println("Connection saved!")

	return nil
}

func encryptPassword(password string) (string, error) {
	key := os.Getenv(keyEnvVar)
	keyBytes := make([]byte, 32)
	copy(keyBytes, []byte(key))
	if key == "" {
		return "", fmt.Errorf("%s is not set", keyEnvVar)
	}

	aesCipher, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %w", err)
	}

	cipherGCM, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return "", fmt.Errorf("error creating new GCM: %w", err)
	}

	nonce := make([]byte, cipherGCM.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", fmt.Errorf("error reading nonce: %w", err)
	}
	encryptedText := cipherGCM.Seal(nonce, nonce, []byte(password), nil)

	return base64.StdEncoding.EncodeToString(encryptedText), nil
}

func decryptPassword(encryptedPassword string) (string, error) {
	key := os.Getenv(keyEnvVar)
	keyBytes := make([]byte, 32)
	copy(keyBytes, []byte(key))
	if key == "" {
		return "", fmt.Errorf("%s is not set", keyEnvVar)
	}

	aesCipher, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("error creating cipher: %w", err)
	}

	cipherGCM, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return "", fmt.Errorf("error creating new GCM: %w", err)
	}

	encryptedPasswordDecoded, err := base64.StdEncoding.DecodeString(encryptedPassword)
	nonce, encryptedText := encryptedPasswordDecoded[:cipherGCM.NonceSize()], encryptedPasswordDecoded[cipherGCM.NonceSize():]

	password, err := cipherGCM.Open(nil, nonce, encryptedText, nil)
	if err != nil {
		return "", fmt.Errorf("error opening encrypted text: %w", err)
	}

	return string(password), nil
}

func main() {
	homeDir := os.Getenv("HOME")
	app := &cli.App{
		Name:  "psqlcm",
		Usage: "psql connection manager",
		Commands: []*cli.Command{
			{
				Name:   "new",
				Usage:  "New connection",
				Action: newConnection,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  flagCacheDir,
						Usage: "Location to store connections",
						Value: fmt.Sprintf("%s/.local/share/psqlcm", homeDir),
					},
					&cli.BoolFlag{
						Name:  flagNotCurrent,
						Usage: "Do not set this new connection as current",
					},
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "List all available connections",
				Action:  list,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  flagCacheDir,
						Usage: "Location to store connections",
						Value: fmt.Sprintf("%s/.local/share/psqlcm", homeDir),
					},
				},
			},
			{
				Name:   "show",
				Usage:  "Show a connection string",
				Action: show,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  flagCacheDir,
						Usage: "Location to store connections",
						Value: fmt.Sprintf("%s/.local/share/psqlcm", homeDir),
					},
				},
			},
			{
				Name:    "delete",
				Usage:   "Remove a cached connection",
				Aliases: []string{"del", "remove"},
				Action:  deleteConnection,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  flagCacheDir,
						Usage: "Location to store connections",
						Value: fmt.Sprintf("%s/.local/share/psqlcm", homeDir),
					},
				},
			},
			{
				Name:  "set-current",
				Usage: "Set a connection as current",
				Action: func(cCtx *cli.Context) error {
					if cCtx.Args().Len() == 0 {
						return fmt.Errorf("pass in the connection to set as current")
					}
					return setCurrent(cCtx.Args().Get(0), cCtx.String(flagCacheDir))
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  flagCacheDir,
						Usage: "Location to store connections",
						Value: fmt.Sprintf("%s/.local/share/psqlcm", homeDir),
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
