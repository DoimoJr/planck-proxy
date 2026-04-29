package veyon

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
)

// AuthType corrisponde all'enum RfbVeyonAuth::Type in core/src/RfbVeyonAuth.h:
//
//	enum Type { Invalid, KeyFile = 3, Logon, Token };
//
// Solo KeyFile e' supportato in questo client (e' l'unico flow non-
// interattivo, adatto a un client headless come Planck).
type AuthType int32

const (
	AuthInvalid AuthType = 0
	AuthKeyFile AuthType = 3
	AuthLogon   AuthType = 4
	AuthToken   AuthType = 5
)

// LoadPrivateKeyPEM legge una chiave privata RSA da PEM. Accetta sia
// "PRIVATE KEY" (PKCS#8, formato Veyon corrente) sia "RSA PRIVATE KEY"
// (PKCS#1, formato legacy). Errore se la chiave non e' RSA.
func LoadPrivateKeyPEM(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("auth: PEM block non trovato")
	}
	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: parse PKCS#8: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("auth: chiave non e' RSA (e' %T)", key)
		}
		return rsaKey, nil
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("auth: tipo PEM non supportato: %q", block.Type)
}

// performKeyFileAuth esegue il flow di auth Veyon KeyFile dopo che il
// security type e' stato scelto. Sequenza (da VeyonConnection.cpp):
//
//  1. Server → Client (VarMsg): [int authTypeCount] [int type_1] ... [int type_N]
//  2. Client → Server (VarMsg): [int chosenAuthType=KeyFile] [string logonUsername]
//  3. Server → Client (VarMsg): [QByteArray challenge]
//  4. Client firma SHA256(challenge) con la sua chiave privata RSA (PKCS#1 v1.5)
//  5. Client → Server (VarMsg): [string keyName] [QByteArray signature]
//  6. Server → Client (RFB SecurityResult, 4 byte): 0=OK, 1=failed
//
// `keyName` e' il nome della chiave (es. "teacher") definito quando le
// chiavi sono state generate via `veyon-cli authkeys create`.
//
// `username` viene scritto come QString al passo 2: in Veyon serve per
// audit log lato server. Vuoto e' accettato per i client privilegiati.
func performKeyFileAuth(rw io.ReadWriter, keyName, username string, key *rsa.PrivateKey) error {
	// Step 1: ricevi lista auth types proposta dal server.
	dec, _, err := RecvVarMsg(rw)
	if err != nil {
		return fmt.Errorf("auth: ricezione auth types: %w", err)
	}
	authTypeCountAny, err := dec.ReadVariant()
	if err != nil {
		return fmt.Errorf("auth: parse authTypeCount: %w", err)
	}
	authTypeCount, ok := authTypeCountAny.(int32)
	if !ok {
		return fmt.Errorf("auth: authTypeCount non e' int (got %T)", authTypeCountAny)
	}
	offered := make([]int32, 0, authTypeCount)
	supports := false
	for i := int32(0); i < authTypeCount; i++ {
		v, err := dec.ReadVariant()
		if err != nil {
			return fmt.Errorf("auth: parse authType[%d]: %w", i, err)
		}
		t, ok := v.(int32)
		if !ok {
			continue
		}
		offered = append(offered, t)
		if AuthType(t) == AuthKeyFile {
			supports = true
		}
	}
	if !supports {
		return fmt.Errorf("auth: server non offre AuthKeyFile (offerti: %v)", offered)
	}

	// Step 2: scegli KeyFile + invia username.
	if err := SendVarMsg(rw, int32(AuthKeyFile), username); err != nil {
		return fmt.Errorf("auth: send chosen + username: %w", err)
	}

	// Step 3: ricevi challenge.
	dec, _, err = RecvVarMsg(rw)
	if err != nil {
		return fmt.Errorf("auth: ricezione challenge: %w", err)
	}
	chAny, err := dec.ReadVariant()
	if err != nil {
		return fmt.Errorf("auth: parse challenge: %w", err)
	}
	challenge, ok := chAny.([]byte)
	if !ok || len(challenge) == 0 {
		return fmt.Errorf("auth: challenge non valido (got %T len %d)", chAny, len(challenge))
	}

	// Step 4: firma SHA512(challenge) con PKCS#1 v1.5.
	// Veyon usa CryptoCore::DefaultSignatureAlgorithm = QCA::EMSA3_SHA512
	// (vedi core/src/CryptoCore.h).
	hashed := sha512Hash(challenge)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, hashed)
	if err != nil {
		return fmt.Errorf("auth: firma RSA: %w", err)
	}

	// Step 5: invia keyName + signature.
	if err := SendVarMsg(rw, keyName, signature); err != nil {
		return fmt.Errorf("auth: send signature: %w", err)
	}

	// Step 6: leggi SecurityResult RFB.
	return rfbReadSecurityResult(rw)
}

// sha512Hash e' un wrapper minimale per il digest SHA-512 usato dalla
// signature di auth Veyon.
func sha512Hash(data []byte) []byte {
	h := sha512.New()
	h.Write(data)
	return h.Sum(nil)
}
