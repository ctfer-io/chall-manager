package fs

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"

	"github.com/ctfer-io/chall-manager/global"
	"go.uber.org/zap"
)

const (
	challSubdir    = "chall"
	instanceSubdir = "instance"
	infoFile       = "info.json"
)

// Hash computes the Hash of the given ID.
// It is used to get a standard identifier (both in size and format)
// while avoiding filesystem manipulation (e.g. path traversal).
func Hash(id string) string {
	h := md5.New()
	_, _ = h.Write([]byte(id))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

func fclose(f *os.File) {
	if err := f.Close(); err != nil {
		global.Log().Error(context.Background(), "failed to close challenge info file", zap.Error(err))
	}
}
