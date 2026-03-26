//go:build darwin

// ABOUTME: macOS system-level keystroke injector using CGEventPost.
// ABOUTME: Posts key events to the focused app or a named target app via CGEventPostToPSN.
package inject

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

// postKeycodeEvent posts a key down + key up event with a specific virtual keycode.
static int postKeycodeEvent(CGKeyCode keyCode, CGEventFlags flags) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, keyCode, true);
    if (down == NULL) return -1;
    if (flags != 0) CGEventSetFlags(down, flags);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);

    CGEventRef up = CGEventCreateKeyboardEvent(NULL, keyCode, false);
    if (up == NULL) return -1;
    if (flags != 0) CGEventSetFlags(up, flags);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);

    return 0;
}

// postUnicodeEvent posts a key down + key up event with a unicode character.
static int postUnicodeEvent(UniChar ch) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (down == NULL) return -1;
    CGEventKeyboardSetUnicodeString(down, 1, &ch);
    CGEventPost(kCGHIDEventTap, down);
    CFRelease(down);

    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (up == NULL) return -1;
    CGEventKeyboardSetUnicodeString(up, 1, &ch);
    CGEventPost(kCGHIDEventTap, up);
    CFRelease(up);

    return 0;
}

// postKeycodeEventToPSN posts a key event to a specific process by PSN.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
static int postKeycodeEventToPSN(CGKeyCode keyCode, CGEventFlags flags, long highPSN, long lowPSN) {
    ProcessSerialNumber psn = {(UInt32)highPSN, (UInt32)lowPSN};

    CGEventRef down = CGEventCreateKeyboardEvent(NULL, keyCode, true);
    if (down == NULL) return -1;
    if (flags != 0) CGEventSetFlags(down, flags);
    CGEventPostToPSN(&psn, down);
    CFRelease(down);

    CGEventRef up = CGEventCreateKeyboardEvent(NULL, keyCode, false);
    if (up == NULL) return -1;
    if (flags != 0) CGEventSetFlags(up, flags);
    CGEventPostToPSN(&psn, up);
    CFRelease(up);

    return 0;
}

// postUnicodeEventToPSN posts a unicode key event to a specific process by PSN.
static int postUnicodeEventToPSN(UniChar ch, long highPSN, long lowPSN) {
    ProcessSerialNumber psn = {(UInt32)highPSN, (UInt32)lowPSN};

    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (down == NULL) return -1;
    CGEventKeyboardSetUnicodeString(down, 1, &ch);
    CGEventPostToPSN(&psn, down);
    CFRelease(down);

    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (up == NULL) return -1;
    CGEventKeyboardSetUnicodeString(up, 1, &ch);
    CGEventPostToPSN(&psn, up);
    CFRelease(up);

    return 0;
}

// getPSNForPID converts a Unix PID to a ProcessSerialNumber.
// Returns 0 on success, non-zero on failure.
static int getPSNForPID(int pid, long *highOut, long *lowOut) {
    ProcessSerialNumber psn;
    OSStatus status = GetProcessForPID((pid_t)pid, &psn);
    if (status != noErr) return -1;
    *highOut = (long)psn.highLongOfPSN;
    *lowOut = (long)psn.lowLongOfPSN;
    return 0;
}
#pragma clang diagnostic pop

// checkAccessibility tests whether we can create a CGEvent (returns 0 on success).
static int checkAccessibility() {
    CGEventRef ev = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (ev == NULL) return -1;
    CFRelease(ev);
    return 0;
}
*/
import "C"

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SystemInjector injects keystrokes at the OS level on macOS using CoreGraphics.
// When targetPID is set, events are posted to that specific process via CGEventPostToPSN.
// When targetPID is 0, events are posted to the focused application.
type SystemInjector struct {
	targetPID int
	psnHigh   C.long
	psnLow    C.long
}

// NewSystem creates a SystemInjector that posts to the focused application.
// Requires Accessibility permissions.
// Grant access in System Settings > Privacy & Security > Accessibility.
func NewSystem() (*SystemInjector, error) {
	if C.checkAccessibility() != 0 {
		return nil, fmt.Errorf(
			"system mode requires Accessibility permissions.\n" +
				"Grant access in System Settings > Privacy & Security > Accessibility",
		)
	}
	return &SystemInjector{}, nil
}

// NewSystemWithTarget creates a SystemInjector that posts to a named application.
// The app must be running. Events are delivered regardless of which app has focus.
func NewSystemWithTarget(appName string) (*SystemInjector, error) {
	if C.checkAccessibility() != 0 {
		return nil, fmt.Errorf(
			"system mode requires Accessibility permissions.\n" +
				"Grant access in System Settings > Privacy & Security > Accessibility",
		)
	}

	pid, err := findAppPID(appName)
	if err != nil {
		return nil, fmt.Errorf("could not find running app %q: %w", appName, err)
	}

	var highPSN, lowPSN C.long
	if C.getPSNForPID(C.int(pid), &highPSN, &lowPSN) != 0 {
		return nil, fmt.Errorf("could not get PSN for app %q (PID %d)", appName, pid)
	}

	return &SystemInjector{
		targetPID: pid,
		psnHigh:   highPSN,
		psnLow:    lowPSN,
	}, nil
}

// findAppPID finds the PID of a running application by name using osascript.
func findAppPID(appName string) (int, error) {
	out, err := exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application "System Events" to unix id of process "%s"`, appName),
	).Output()
	if err != nil {
		return 0, fmt.Errorf("app %q is not running or not found", appName)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("unexpected PID output: %q", string(out))
	}
	return pid, nil
}

// Inject delivers raw keystroke bytes via CGEventPost (untargeted) or
// CGEventPostToPSN (targeted).
func (s *SystemInjector) Inject(data []byte) error {
	events := parseKeyEvents(data)
	for _, ev := range events {
		var err error
		if s.targetPID != 0 {
			err = s.postKeyEventTargeted(ev)
		} else {
			err = postKeyEvent(ev)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// HasOutput returns false because system mode has no output stream.
func (s *SystemInjector) HasOutput() bool {
	return false
}

// Close is a no-op for the system injector.
func (s *SystemInjector) Close() error {
	return nil
}

// letterKeycodes maps lowercase letters to macOS virtual keycodes.
var letterKeycodes = map[rune]uint16{
	'a': 0x00, 'b': 0x0B, 'c': 0x08, 'd': 0x02, 'e': 0x0E, 'f': 0x03,
	'g': 0x05, 'h': 0x04, 'i': 0x22, 'j': 0x26, 'k': 0x28, 'l': 0x25,
	'm': 0x2E, 'n': 0x2D, 'o': 0x1F, 'p': 0x23, 'q': 0x0C, 'r': 0x0F,
	's': 0x01, 't': 0x11, 'u': 0x20, 'v': 0x09, 'w': 0x0D, 'x': 0x07,
	'y': 0x10, 'z': 0x06,
}

// postKeyEvent posts a single key event to the focused app via CGEventPost.
func postKeyEvent(ev keyEvent) error {
	switch ev.eventType {
	case keyEventUnicode:
		if C.postUnicodeEvent(C.UniChar(ev.char)) != 0 {
			return fmt.Errorf("failed to post unicode event for %q", ev.char)
		}
	case keyEventKeycode:
		if C.postKeycodeEvent(C.CGKeyCode(ev.keycode), 0) != 0 {
			return fmt.Errorf("failed to post keycode event for keycode %d", ev.keycode)
		}
	case keyEventControl:
		kc, ok := letterKeycodes[ev.char]
		if !ok {
			// Fallback: post the raw control character as unicode
			controlChar := ev.char - 0x60 // 'a' -> 0x01, etc.
			if C.postUnicodeEvent(C.UniChar(controlChar)) != 0 {
				return fmt.Errorf("failed to post control event for %q", ev.char)
			}
			return nil
		}
		flags := C.CGEventFlags(C.kCGEventFlagMaskControl)
		if C.postKeycodeEvent(C.CGKeyCode(kc), flags) != 0 {
			return fmt.Errorf("failed to post control event for %q", ev.char)
		}
	}
	return nil
}

// postKeyEventTargeted posts a single key event to the target process via CGEventPostToPSN.
func (s *SystemInjector) postKeyEventTargeted(ev keyEvent) error {
	switch ev.eventType {
	case keyEventUnicode:
		if C.postUnicodeEventToPSN(C.UniChar(ev.char), s.psnHigh, s.psnLow) != 0 {
			return fmt.Errorf("failed to post unicode event for %q to PID %d", ev.char, s.targetPID)
		}
	case keyEventKeycode:
		if C.postKeycodeEventToPSN(C.CGKeyCode(ev.keycode), 0, s.psnHigh, s.psnLow) != 0 {
			return fmt.Errorf("failed to post keycode event for keycode %d to PID %d", ev.keycode, s.targetPID)
		}
	case keyEventControl:
		kc, ok := letterKeycodes[ev.char]
		if !ok {
			controlChar := ev.char - 0x60
			if C.postUnicodeEventToPSN(C.UniChar(controlChar), s.psnHigh, s.psnLow) != 0 {
				return fmt.Errorf("failed to post control event for %q to PID %d", ev.char, s.targetPID)
			}
			return nil
		}
		flags := C.CGEventFlags(C.kCGEventFlagMaskControl)
		if C.postKeycodeEventToPSN(C.CGKeyCode(kc), flags, s.psnHigh, s.psnLow) != 0 {
			return fmt.Errorf("failed to post control event for %q to PID %d", ev.char, s.targetPID)
		}
	}
	return nil
}
