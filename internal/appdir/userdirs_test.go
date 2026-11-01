package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

// userDirsZH is user-dirs.dirs as xdg-user-dirs-update writes it on a
// Chinese desktop (Ubuntu 24.04, zh_CN.UTF-8): Downloads is ~/下载, not
// ~/Downloads (1Shin on Discord: an export joined $HOME/Downloads).
const userDirsZH = `# This file is written by xdg-user-dirs-update
# If you want to change or add directories, just edit the line you're
# interested in. All local changes will be retained on the next run.
# Format is XDG_xxx_DIR="$HOME/yyy", where yyy is a shell-escaped
# homedir-relative path, or XDG_xxx_DIR="/yyy", where /yyy is an
# absolute path. No other format is supported.
# 
XDG_DESKTOP_DIR="$HOME/桌面"
XDG_DOWNLOAD_DIR="$HOME/下载"
XDG_TEMPLATES_DIR="$HOME/模板"
XDG_PUBLICSHARE_DIR="$HOME/公共的"
XDG_DOCUMENTS_DIR="$HOME/文档"
XDG_MUSIC_DIR="$HOME/音乐"
XDG_PICTURES_DIR="$HOME/图片"
XDG_VIDEOS_DIR="$HOME/视频"
`

func userDirsHome(t *testing.T, dirs string) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	if dirs != "" {
		os.MkdirAll(filepath.Join(h, ".config"), 0o700)
		if err := os.WriteFile(filepath.Join(h, ".config", "user-dirs.dirs"), []byte(dirs), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func fakeKnownFolder(t *testing.T, f func(kind string) (string, string)) {
	t.Helper()
	was := knownFolder
	knownFolder = f
	t.Cleanup(func() { knownFolder = was })
}

func TestUserFolderXDG(t *testing.T) {
	h := userDirsHome(t, userDirsZH)
	for kind, want := range map[string]string{
		DownloadsFolder: filepath.Join(h, "下载"),
		DesktopFolder:   filepath.Join(h, "桌面"),
		DocumentsFolder: filepath.Join(h, "文档"),
	} {
		if got := userFolderOn("linux", h, kind); got != want {
			t.Errorf("linux %s: %q, want %q", kind, got, want)
		}
		if got := userFolderOn("freebsd", h, kind); got != want {
			t.Errorf("freebsd %s: %q, want %q", kind, got, want)
		}
	}
	// A Mac's Downloads can't be moved: a user-dirs.dirs there is no one's.
	if got, want := userFolderOn("darwin", h, DownloadsFolder), filepath.Join(h, "Downloads"); got != want {
		t.Errorf("darwin: %q, want %q", got, want)
	}
}

func TestUserFolderXDGLines(t *testing.T) {
	for _, c := range []struct {
		name, dirs, want string
	}{
		{"absolute", `XDG_DOWNLOAD_DIR="/mnt/data/dl"`, "/mnt/data/dl"},
		{"escaped", `XDG_DOWNLOAD_DIR="$HOME/my \"dl\""`, "~/my \"dl\""},
		{"spaces round =", `  XDG_DOWNLOAD_DIR = "$HOME/dl"`, "~/dl"},
		{"turned off", `XDG_DOWNLOAD_DIR="$HOME/"`, "~"},
		{"home itself", `XDG_DOWNLOAD_DIR="$HOME"`, "~"},
		{"the last wins", "XDG_DOWNLOAD_DIR=\"$HOME/a\"\nXDG_DOWNLOAD_DIR=\"$HOME/b\"", "~/b"},
		{"commented", `# XDG_DOWNLOAD_DIR="$HOME/a"`, "~/Downloads"},
		{"unquoted", `XDG_DOWNLOAD_DIR=$HOME/a`, "~/Downloads"},
		{"relative", `XDG_DOWNLOAD_DIR="a/b"`, "~/Downloads"},
		{"another variable", `XDG_DOWNLOAD_DIR="$XDG_DATA_HOME/a"`, "~/Downloads"},
		{"another folder only", `XDG_MUSIC_DIR="$HOME/a"`, "~/Downloads"},
		{"empty file", "\n", "~/Downloads"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := userDirsHome(t, c.dirs)
			want := c.want
			if want[0] == '/' {
				want = filepath.FromSlash(want)
			} else if want == "~" {
				want = h
			} else if len(want) > 1 && want[:2] == "~/" {
				want = filepath.Join(h, filepath.FromSlash(want[2:]))
			}
			if got := userFolderOn("linux", h, DownloadsFolder); got != want {
				t.Errorf("%q, want %q", got, want)
			}
		})
	}
}

// With no user-dirs.dirs, or XDG_CONFIG_HOME unset (home/.config is read),
// or relative (ignored, as the spec asks).
func TestUserFolderXDGConfigHome(t *testing.T) {
	h := userDirsHome(t, "")
	if got, want := userFolderOn("linux", h, DownloadsFolder), filepath.Join(h, "Downloads"); got != want {
		t.Errorf("no file: %q, want %q", got, want)
	}
	userDirsHome2 := func(cfg string) {
		os.MkdirAll(filepath.Join(h, ".config"), 0o700)
		os.WriteFile(filepath.Join(h, ".config", "user-dirs.dirs"), []byte(`XDG_DOWNLOAD_DIR="$HOME/dl"`), 0o600)
		t.Setenv("XDG_CONFIG_HOME", cfg)
	}
	for _, cfg := range []string{"", "rel/cfg"} {
		userDirsHome2(cfg)
		if got, want := userFolderOn("linux", h, DownloadsFolder), filepath.Join(h, "dl"); got != want {
			t.Errorf("XDG_CONFIG_HOME %q: %q, want %q", cfg, got, want)
		}
	}
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "user-dirs.dirs"), []byte(`XDG_DOWNLOAD_DIR="$HOME/other"`), 0o600)
	t.Setenv("XDG_CONFIG_HOME", other)
	if got, want := userFolderOn("linux", h, DownloadsFolder), filepath.Join(h, "other"); got != want {
		t.Errorf("XDG_CONFIG_HOME set: %q, want %q", got, want)
	}
}

// Windows' Downloads is the known folder (moved to D:\ or OneDrive), when
// magpie's home is the profile Windows keeps it for.
func TestUserFolderWindows(t *testing.T) {
	h := userDirsHome(t, userDirsZH) // not read on Windows
	moved := filepath.Join(t.TempDir(), "OneDrive", "Downloads")
	fakeKnownFolder(t, func(kind string) (string, string) {
		if kind != DownloadsFolder {
			return "", ""
		}
		return moved, h
	})
	if got := userFolderOn("windows", h, DownloadsFolder); got != moved {
		t.Errorf("moved: %q, want %q", got, moved)
	}
	if got, want := userFolderOn("windows", h, DesktopFolder), filepath.Join(h, "Desktop"); got != want {
		t.Errorf("lookup failed: %q, want %q", got, want)
	}
	// Another home's known folder: magpie's own home keeps home\Downloads.
	other := t.TempDir()
	if got, want := userFolderOn("windows", other, DownloadsFolder), filepath.Join(other, "Downloads"); got != want {
		t.Errorf("another home: %q, want %q", got, want)
	}
	fakeKnownFolder(t, func(string) (string, string) { return moved, "" })
	if got, want := userFolderOn("windows", h, DownloadsFolder), filepath.Join(h, "Downloads"); got != want {
		t.Errorf("no profile: %q, want %q", got, want)
	}
}

// A data folder beside magpie in a moved Downloads isn't its portable one
// either (tangle778's case, in the folder the system keeps).
func TestSharedFolderMovedDownloads(t *testing.T) {
	h := userDirsHome(t, userDirsZH)
	moved := filepath.Join(h, "下载")
	fakeKnownFolder(t, func(kind string) (string, string) {
		if kind == DownloadsFolder {
			return moved, h
		}
		return "", ""
	})
	if UserFolder(DownloadsFolder) == filepath.Join(h, "Downloads") {
		t.Skip("this system's Downloads can't be moved")
	}
	if !sharedFolder(moved) {
		t.Errorf("%s not shared", moved)
	}
	if !sharedFolder(filepath.Join(h, "Downloads")) {
		t.Error("home's Downloads not shared")
	}
	if sharedFolder(filepath.Join(moved, "magpie")) {
		t.Error("a folder inside shared")
	}
}
