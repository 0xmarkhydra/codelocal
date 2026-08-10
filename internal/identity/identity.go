package identity

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Credential struct {
	CredentialID     string `json:"credentialId"`
	CredentialSecret string `json:"credentialSecret"`
	DeviceID         string `json:"deviceId"`
	DeviceName       string `json:"deviceName"`
	ServerURL        string `json:"serverUrl"`
	CreatedAt        int64  `json:"createdAt"`
}

func credentialFile() string {
	if value := os.Getenv("CODELOCAL_CREDENTIAL_FILE"); value != "" {
		return value
	}
	return filepath.Join(state.Dir(), "device-credential.json")
}

func Load(serverURL string) (*Credential, error) {
	var out Credential
	if err := state.ReadJSON(credentialFile(), &out); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if serverURL != "" && strings.TrimRight(out.ServerURL, "/") != strings.TrimRight(serverURL, "/") {
		return nil, nil
	}
	return &out, nil
}

func Save(value Credential) error {
	if value.CreatedAt == 0 {
		value.CreatedAt = time.Now().UnixMilli()
	}
	return state.WriteJSONAtomic(credentialFile(), value)
}

func Delete() error {
	err := os.Remove(credentialFile())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func Device() (id, name string) {
	host, _ := os.Hostname()
	id = os.Getenv("CODELOCAL_DEVICE_ID")
	if id == "" {
		id = host
	}
	name = os.Getenv("CODELOCAL_DEVICE_NAME")
	if name == "" {
		name = host
	}
	return id, name
}
