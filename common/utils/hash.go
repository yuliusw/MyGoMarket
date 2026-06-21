// common/utils/hash.go
package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// 参数配置：针对低延迟优化
const (
	timeParams    = 1         // 迭代次数
	memoryParams  = 32 * 1024 // 32MB 内存
	threadsParams = 2         // 并行度
	keyLen        = 32
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, timeParams, memoryParams, threadsParams, keyLen)

	// 格式: $argon2id$v=19$m=32768,t=1,p=2$salt$hash
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryParams, timeParams, threadsParams, b64Salt, b64Hash), nil
}

func CheckPasswordHash(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) < 6 {
		return false
	}

	var m, t, p uint32
	fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p)

	salt, _ := base64.RawStdEncoding.DecodeString(parts[4])
	decodedHash, _ := base64.RawStdEncoding.DecodeString(parts[5])

	comparisonHash := argon2.IDKey([]byte(password), salt, t, m, uint8(p), uint32(len(decodedHash)))

	return subtle.ConstantTimeCompare(decodedHash, comparisonHash) == 1
}
