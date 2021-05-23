package utils

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"

	"github.com/libp2p/go-libp2p-core/crypto"
)

func LoadKey(keyFile string, keyReset bool) crypto.PrivKey {
	var prvKey crypto.PrivKey
	var err error

	if keyReset {
		// generate new key
		fmt.Println("[KEY UTIL] Generating a new key")
		prvKey = SafeKeyGen()
		SaveKey(keyFile, prvKey)
		return prvKey
	}

	if keyFile == "" {
		fmt.Println("[KEY UTIL] Using a one time key")
		prvKey = SafeKeyGen()
		return prvKey
	}

	// load from file
	data, err := ioutil.ReadFile(keyFile)
	if err != nil {
		fmt.Printf("[KEY UTIL] Key could not be read from file '%s' - %s\n", keyFile, err)
		prvKey = SafeKeyGen()
		SaveKey(keyFile, prvKey)
		return prvKey
	}

	// unpack data
	prvKey, err = crypto.UnmarshalPrivateKey(data)
	if err != nil {
		fmt.Printf("[KEY UTIL] Key could not be decoded - %s\n", err)
		prvKey = SafeKeyGen()
		SaveKey(keyFile, prvKey)
		return prvKey
	}

	// key was loaded okay, no need to save it
	return prvKey
}

func SafeKeyGen() crypto.PrivKey {
	r := rand.Reader
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		fmt.Printf("[KEY UTIL] Error generating key - %s\n", err)
		return nil
	}
	return prvKey
}

func SaveKey(keyFile string, prvKey crypto.PrivKey) {
	// do not save null key
	if prvKey == nil {
		return
	}

	fmt.Printf("[KEY UTIL] Saving key to file '%s'\n", keyFile)

	// marshal the key
	marshaledKey, err := crypto.MarshalPrivateKey(prvKey)
	if err != nil {
		fmt.Println("[KEY UTIL] Key saving failed:", err)
		return
	}

	// save new key to file
	// TODO: change file permissions
	err = ioutil.WriteFile(keyFile, marshaledKey, 0777)
	if err != nil {
		fmt.Println("[KEY UTIL] Key saving failed:", err)
		return
	}
}
