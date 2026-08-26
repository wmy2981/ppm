// Package elevate handles UAC self-elevation: relaunching the process as
// administrator when needed.
package elevate

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the current process runs with administrator
// rights.
func IsElevated() bool {
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

var (
	modshell32       = windows.NewLazySystemDLL("shell32.dll")
	modUser32        = windows.NewLazySystemDLL("user32.dll")
	procShellExecute = modshell32.NewProc("ShellExecuteW")
	procMessageBox   = modUser32.NewProc("MessageBoxW")
)

const swShow = 5 // SW_SHOW

// ElevateOrExit re-launches itself with "runas" via ShellExecuteW when not
// elevated. The parent then exits: success means the elevated child took
// over; a declined UAC prompt or launch failure exits with an error dialog.
func ElevateOrExit() {
	if IsElevated() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		Fatal("cannot locate executable: %v", err)
	}
	cwd, _ := os.Getwd()

	verb, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	var cwdPtr *uint16
	if cwd != "" {
		cwdPtr, _ = syscall.UTF16PtrFromString(cwd)
	}

	ret, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exePtr)),
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		swShow,
	)
	if ret > 32 {
		os.Exit(0) // elevated child launched; this instance is done
	}
	const errCancelled = syscall.Errno(1223) // ERROR_CANCELLED
	if callErr != nil && callErr == errCancelled {
		fmt.Fprintln(os.Stderr, "UAC prompt declined, exiting.")
		os.Exit(1)
	}
	msg := fmt.Sprintf("elevation failed (code %d)", ret)
	if callErr != nil {
		msg += ": " + callErr.Error()
	}
	Fatal("%s", msg)
}

// Fatal shows an error MessageBox (so double-click users see it), prints to
// stderr, waits for Enter, and exits.
func Fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	MessageBoxError(msg)
	fmt.Fprintf(os.Stderr, "\nerror: %s\nPress Enter to exit...", msg)
	fmt.Scanln()
	os.Exit(1)
}

func MessageBoxError(msg string) {
	title, err1 := syscall.UTF16PtrFromString("ppm")
	text, err2 := syscall.UTF16PtrFromString(strings.ReplaceAll(msg, "\n", "\r\n"))
	if err1 != nil || err2 != nil || procMessageBox.Find() != nil {
		return
	}
	procMessageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10 /*MB_ICONERROR*/)
}
