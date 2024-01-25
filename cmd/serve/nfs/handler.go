//go:build unix
// +build unix

package nfs

import (
	"context"
	"fmt"
	"net"

	"github.com/go-git/go-billy/v5"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/vfs"
	"github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
)

// NewBackendAuthHandler creates a handler for the provided filesystem
func NewBackendAuthHandler(vfs *vfs.VFS) nfs.Handler {
	handler := &BackendAuthHandler{
		vfs: vfs,
	}
	handler.cache = cacheHelper(handler, handler.HandleLimit())
	return handler
}

// BackendAuthHandler returns a NFS backing that exposes a given file system in response to all mount requests.
type BackendAuthHandler struct {
	vfs   *vfs.VFS
	cache nfs.Handler
}

// Mount backs Mount RPC Requests, allowing for access control policies.
func (h *BackendAuthHandler) Mount(ctx context.Context, conn net.Conn, req nfs.MountRequest) (status nfs.MountStatus, hndl billy.Filesystem, auths []nfs.AuthFlavor) {
	status = nfs.MountStatusOk
	hndl = &FS{vfs: h.vfs}
	auths = []nfs.AuthFlavor{nfs.AuthFlavorNull}
	return
}

// Change provides an interface for updating file attributes.
func (h *BackendAuthHandler) Change(fs billy.Filesystem) billy.Change {
	if c, ok := fs.(billy.Change); ok {
		return c
	}
	return nil
}

// FSStat provides information about a filesystem.
func (h *BackendAuthHandler) FSStat(ctx context.Context, f billy.Filesystem, s *nfs.FSStat) error {
	total, _, free := h.vfs.Statfs()
	s.TotalSize = uint64(total)
	s.FreeSize = uint64(free)
	s.AvailableSize = uint64(free)
	return nil
}

// ToHandle handled by CachingHandler
func (h *BackendAuthHandler) ToHandle(f billy.Filesystem, s []string) []byte {
	return h.cache.ToHandle(f, s)
}

// FromHandle handled by CachingHandler
func (h *BackendAuthHandler) FromHandle(b []byte) (billy.Filesystem, []string, error) {
	return h.cache.FromHandle(b)
}

// HandleLimit handled by cachingHandler
func (h *BackendAuthHandler) HandleLimit() int {
	if h.vfs.Opt.CacheMaxSize < 0 {
		return 1000000
	}
	if h.vfs.Opt.CacheMaxSize <= 5 {
		return 5
	}
	return int(h.vfs.Opt.CacheMaxSize)
}

// InvalidateHandle is called on removes or renames
func (h *BackendAuthHandler) InvalidateHandle(billy.Filesystem, []byte) error {
	return nil
}

func newHandler(vfs *vfs.VFS) nfs.Handler {
	handler := NewBackendAuthHandler(vfs)
	nfs.SetLogger(&NFSLogIntercepter{Level: nfs.DebugLevel})
	return handler
}

func cacheHelper(handler nfs.Handler, limit int) nfs.Handler {
	cacheHelper := nfshelper.NewCachingHandler(handler, limit)
	return cacheHelper
}

/* intercept noisy go-nfs logs and reroute them to DEBUG */

type NFSLogIntercepter struct {
	Level nfs.LogLevel
}

func (l *NFSLogIntercepter) Intercept(args ...interface{}) {
	args = append([]interface{}{"[NFS DEBUG] "}, args...)
	argsS := fmt.Sprint(args...)
	fs.Debugf(nil, "%v", argsS)
}
func (l *NFSLogIntercepter) Interceptf(format string, args ...interface{}) {
	fs.Debugf(nil, "[NFS DEBUG] "+format, args...)
}
func (l *NFSLogIntercepter) Debug(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Debugf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) Error(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Errorf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) Fatal(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Fatalf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) GetLevel() nfs.LogLevel {
	return l.Level
}
func (l *NFSLogIntercepter) Info(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Infof(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) Panic(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Panicf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) ParseLevel(level string) (nfs.LogLevel, error) {
	return nfs.Log.ParseLevel(level)
}
func (l *NFSLogIntercepter) Print(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Printf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) SetLevel(level nfs.LogLevel) {
	l.Level = level
}
func (l *NFSLogIntercepter) Trace(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Tracef(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
func (l *NFSLogIntercepter) Warn(args ...interface{}) {
	l.Intercept(args...)
}
func (l *NFSLogIntercepter) Warnf(format string, args ...interface{}) {
	l.Interceptf(format, args...)
}
