package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"

	"github.com/justmamadou/Kodi/internal/apperr"
	"golang.org/x/crypto/argon2"
)

// Paramètres argon2id recommandés par l'OWASP pour un serveur à mémoire modeste (47 Mio, t=1, p=1).
const (
	argonMemory  = 47104
	argonTime    = 1
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword produit un hash argon2id au format PHC ; le mot de passe n'est jamais stocké en clair.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword compare en temps constant. Les paramètres sont relus du hash, ce qui permet
// de les renforcer plus tard sans invalider les comptes existants.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || threads == 0 || iterations == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	if len(want) != argonKeyLen {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, argonKeyLen)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ValidatePassword applique la règle de la maquette d'inscription : 10 caractères dont un chiffre.
func ValidatePassword(v *apperr.Validator, password string) {
	v.Check(len(password) >= 10, "password", "Le mot de passe doit contenir au moins 10 caractères.")
	v.Check(len(password) <= 256, "password", "Le mot de passe est trop long.")
	v.Check(strings.ContainsFunc(password, unicode.IsDigit), "password", "Le mot de passe doit contenir au moins un chiffre.")
}

// NewToken génère un jeton opaque de 256 bits et son hash SHA-256 : seul le hash va en base.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken hache un jeton reçu du client.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
