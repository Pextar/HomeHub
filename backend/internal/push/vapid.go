// Package push manages Web Push notifications: VAPID key management,
// subscription storage, and notification dispatch.
package push

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// VAPIDKeys holds the ECDH key pair used to authenticate push messages sent
// by this server (VAPID = Voluntary Application Server Identification).
// They are generated once and stored persistently in data/vapid.json.
type VAPIDKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

const vapidFile = "vapid.json"

// LoadOrGenerateVAPIDKeys reads data/vapid.json; if the file does not
// exist yet, a fresh key pair is generated, saved, and returned.
func LoadOrGenerateVAPIDKeys(dataDir string) (*VAPIDKeys, error) {
	path := filepath.Join(dataDir, vapidFile)

	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		var k VAPIDKeys
		if err := json.NewDecoder(f).Decode(&k); err == nil && k.PublicKey != "" {
			return &k, nil
		}
	}

	// Generate a new key pair.
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("generate VAPID keys: %w", err)
	}
	k := &VAPIDKeys{PublicKey: pub, PrivateKey: priv}

	// Persist so we reuse the same keys across restarts (subscriptions are
	// bound to the public key; regenerating it invalidates all existing subs).
	if err := writeVAPIDJSON(path, k); err != nil {
		return nil, fmt.Errorf("save VAPID keys: %w", err)
	}
	return k, nil
}

func writeVAPIDJSON(path string, v interface{}) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(v); encErr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return encErr
	}
	if closeErr := tmp.Close(); closeErr != nil {
		os.Remove(tmpName)
		return closeErr
	}
	return os.Rename(tmpName, path)
}
