package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/urfave/cli/v2"
)

type connection struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Database  string `json:"database"`
	User      string `json:"user"`
	Password  string `json:"password"`
	IsCurrent bool   `json:"is_current"`
}

func (c connection) String() string {
	connectionString := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s", c.User, c.Password, c.Host, c.Port, c.Database)
	return connectionString
}

const (
	flagCacheDir   string = "cache-dir"
	flagNotCurrent string = "not-current"

	defaultHostname string = "localhost"
	defaultPort     int    = 5432
	defaultUser     string = "postgres"
	defaultDatabase string = "postgres"

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

func list(cCtx *cli.Context) error {
	dirs, err := os.ReadDir(cCtx.String(flagCacheDir))
	if err != nil {
		return fmt.Errorf("error reading cache dir: %w", err)
	}

	for _, dir := range dirs {
		fmt.Println(dir.Name())
	}
	return nil
}

func generateConnectionName() string {
	now := time.Now()
	return fmt.Sprintf("pg%v", now.UnixMilli())
}

func setCurrent(c *connection, connectionName, path string) error {
	src := filepath.Join(path, connectionName)
	dst := filepath.Join(path, currentConnectionName)

	if _, err := os.Stat(dst); err == nil {
		os.Remove(dst)
	}

	return os.Symlink(src, dst)
}

func login(cCtx *cli.Context) error {
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

	fmt.Printf("🔒 Password: ")
	var password string
	fmt.Scanln(&password)
	newConnection.Password, err = encryptPassword(password)
	if err != nil {
		return fmt.Errorf("error encrypting password: %w", err)
	}

	defaultConnectionName := generateConnectionName()
	fmt.Printf("📕 Connection name [%s]: ", defaultConnectionName)
	var connectionName string
	fmt.Scanln(&connectionName)
	if connectionName == "" {
		connectionName = defaultConnectionName
	}

	_, err = os.Stat(cCtx.String(flagCacheDir))
	if err != nil {
		err = os.Mkdir(cCtx.String(flagCacheDir), os.ModePerm)
		if err != nil {
			return fmt.Errorf("error making output cache dir: %w", err)
		}
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
		if err := setCurrent(newConnection, connectionName, cCtx.String(flagCacheDir)); err != nil {
			return fmt.Errorf("error setting connection as current: %w", err)
		}
	}

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
				Name:   "login",
				Usage:  "Login and save credentials",
				Action: login,
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
				Name:   "delete",
				Usage:  "Remove a cached connection",
				Action: deleteConnection,
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
