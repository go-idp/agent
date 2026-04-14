package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-zoox/zoox"
)

func appendFileAPI() func(ctx *zoox.Context) {
	return func(ctx *zoox.Context) {
		rawPath := ctx.Query().Get("path").String()
		if rawPath == "" {
			ctx.Fail(fmt.Errorf("path is required"), 400, "path is required")
			return
		}

		cleanPath := filepath.Clean(rawPath)
		if !filepath.IsAbs(cleanPath) {
			ctx.Fail(fmt.Errorf("path must be absolute"), 400, "path must be absolute")
			return
		}
		if strings.HasSuffix(cleanPath, string(os.PathSeparator)) {
			ctx.Fail(fmt.Errorf("path must be file path"), 400, "path must be file path")
			return
		}

		parent := filepath.Dir(cleanPath)
		if st, err := os.Stat(parent); err != nil || !st.IsDir() {
			ctx.Fail(fmt.Errorf("parent directory not found"), 400, "parent directory not found")
			return
		}

		truncate := ctx.Query().Get("truncate").Bool()
		flag := os.O_CREATE | os.O_WRONLY | os.O_APPEND
		if truncate {
			flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		}

		f, err := os.OpenFile(cleanPath, flag, 0644)
		if err != nil {
			ctx.Fail(fmt.Errorf("failed to open file: %s", err), 500, "failed to open file")
			return
		}
		defer f.Close()

		written, err := io.Copy(f, ctx.Request.Body)
		if err != nil {
			ctx.Fail(fmt.Errorf("failed to write file: %s", err), 500, "failed to write file")
			return
		}

		ctx.Success(zoox.H{
			"path":  cleanPath,
			"size":  written,
			"mode":  "append",
			"reset": truncate,
		})
	}
}

