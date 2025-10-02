// main.go
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"go.bug.st/serial"
)

const (
	defaultProgrammer = "stk500v1"
	defaultChip       = "at89s51"
)

// TappableSelect wraps widget.Select to run a handler when the select is tapped
type TappableSelect struct {
	widget.Select
	OnTapped func()
}

// Tapped will call custom OnTapped and then call the embedded Select's Tapped to propagate.
func (m *TappableSelect) Tapped(pe *fyne.PointEvent) {
	if m.OnTapped != nil {
		// let the user refresh options
		m.OnTapped()
	}
	// propagate to embedded Select so it opens and animates as normal
	m.Select.Tapped(pe)
}

// NewTappableSelect creates and initializes a TappableSelect safely.
func NewTappableSelect(options []string, onChanged func(string)) *TappableSelect {
	p := &TappableSelect{}
	p.SetOptions(options)
	p.OnChanged = onChanged
	// ensure the widget base is initialized for wrapper
	p.ExtendBaseWidget(p)
	return p
}

// PassiveEntry forwards scroll wheel events to a parent scroll container
// so that window scrolling works even when the cursor is over a text field.
type PassiveEntry struct {
	widget.Entry
	ParentScroll *container.Scroll
}

func NewPassiveEntry() *PassiveEntry {
	p := &PassiveEntry{}
	p.ExtendBaseWidget(p)
	return p
}

// TODO: fix
func (p *PassiveEntry) Scrolled(ev *fyne.ScrollEvent) {
	println("Here")
	if p.ParentScroll != nil {
		println("wtf")
		p.ParentScroll.Scrolled(ev)
	}
}

type ReadOnlyMultilineEntry struct {
	widget.Entry
}

func (entry *ReadOnlyMultilineEntry) MouseUp(event *desktop.MouseEvent) {
}
func (entry *ReadOnlyMultilineEntry) MouseDown(event *desktop.MouseEvent) {
}
func (entry *ReadOnlyMultilineEntry) TouchUp(event *mobile.TouchEvent) {
}
func (entry *ReadOnlyMultilineEntry) TouchDown(event *mobile.TouchEvent) {
}

func NewReadOnlyMultiLineEntry() *ReadOnlyMultilineEntry {
	entry := &ReadOnlyMultilineEntry{}
	entry.MultiLine = true
	entry.ExtendBaseWidget(entry)
	return entry
}

func main() {
	myApp := app.NewWithID("8051prog")
	w := myApp.NewWindow("8051 Programmer")
	w.Resize(fyne.NewSize(900, 720))
	// allow user resizing/maximize
	// Programmer & Chip
	progSelect := widget.NewSelect([]string{defaultProgrammer}, nil)
	progSelect.SetSelected(defaultProgrammer)
	chipSelect := widget.NewSelect([]string{defaultChip}, nil)
	chipSelect.SetSelected(defaultChip)

	progChipRow := container.NewGridWithColumns(2,
		container.NewVBox(widget.NewLabelWithStyle("Programmer", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), progSelect),
		container.NewVBox(widget.NewLabelWithStyle("Chip", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), chipSelect),
	)

	// Serial ports dropdown (tappable to refresh list)
	serialDropdown := NewTappableSelect([]string{}, nil)
	serialDropdown.PlaceHolder = "Select serial port"
	serialDropdown.OnChanged = func(s string) {
		if s == "<no ports found>" {
			serialDropdown.Selected = ""
		}
	}
	serialDropdown.OnTapped = func() {
		// Update available serial ports each time the control is opened
		ports, err := serial.GetPortsList()
		if err != nil || len(ports) == 0 {
			ports = []string{"<no ports found>"}
		}
		serialDropdown.Options = []string{}
		for _, port := range ports {
			port = strings.TrimSpace(port)
			// MacOS port filtering
			if runtime.GOOS != "darwin" || (!strings.HasPrefix(port, "/dev/cu") && !strings.HasSuffix(port, "debug") || !strings.HasSuffix(port, "console") || !strings.HasSuffix(port, "Bluetooth-Incoming-Port")) {
				serialDropdown.Options = append(serialDropdown.Options, port)
			}
		}
		serialDropdown.Refresh()
	}

	// hex file entry + choose button
	hexEntry := NewPassiveEntry()
	hexEntry.SetPlaceHolder("Path to .hex file for flashing or reading")
	chooseHexBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err == nil && r != nil {
				hexEntry.SetText(r.URI().Path())
				r.Close()
			}
		}, w)
		d.Show()
	})
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		hexEntry.SetText(uris[0].Path())
	})
	removeHexBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		hexEntry.SetText("")
	})
	chooseHexBtn.Importance = widget.LowImportance // make them small/flat
	removeHexBtn.Importance = widget.LowImportance
	hexRow := container.NewBorder(nil, nil, nil, container.NewHBox(removeHexBtn, chooseHexBtn), hexEntry)

	// Output area: read-only multiline entry with both-axis scrolling and no wrap
	outputBox := NewReadOnlyMultiLineEntry()
	outputBox.Wrapping = fyne.TextWrapOff
	outputBox.TextStyle = fyne.TextStyle{Monospace: true}
	outputBox.SetMinRowsVisible(20)

	// Advanced options widgets
	forceChk := widget.NewCheck("Force (-F)", nil)
	disableVerifyChk := widget.NewCheck("Disable verify (-V)", nil)
	disableEraseChk := widget.NewCheck("Disable flash erase (-D)", nil)
	eraseEEPROMChk := widget.NewCheck("Erase flash and EEPROM (-e)", nil)
	doNotWriteChk := widget.NewCheck("Do not write (-n)", nil)
	readToFileChk := widget.NewCheck("Read flash to selected file", nil)

	verbosity := widget.NewSelect([]string{"0", "1", "2", "3", "4"}, nil)
	verbosity.SetSelected("0")

	baudEntry := NewPassiveEntry()
	baudEntry.SetPlaceHolder("19200")
	baudEntry.SetText("19200")

	confEntry := NewPassiveEntry()
	confEntry.SetPlaceHolder("Custom avrdude.conf (optional)")
	chooseConfBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err == nil && r != nil {
				confEntry.SetText(r.URI().Path())
				r.Close()
			}
		}, w)
		d.Show()
	})
	removeConfBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		confEntry.SetText("")
	})
	chooseConfBtn.Importance = widget.LowImportance
	removeConfBtn.Importance = widget.LowImportance
	confRow := container.NewBorder(nil, nil, nil, container.NewHBox(removeConfBtn, chooseConfBtn), confEntry)

	advancedGrid := container.NewVBox(
		container.NewGridWithColumns(2, forceChk, eraseEEPROMChk),
		container.NewGridWithColumns(2, disableVerifyChk, doNotWriteChk),
		container.NewGridWithColumns(2, disableEraseChk, readToFileChk),
		container.NewGridWithColumns(2, widget.NewLabel("Verbosity:"), verbosity),
		container.NewGridWithColumns(2, widget.NewLabel("Baudrate:"), baudEntry),
		confRow,
	)
	advancedAccordion := widget.NewAccordion(widget.NewAccordionItem("Advanced Options", advancedGrid))

	// Buttons (use state values when building args)
	flashBtn := widget.NewButton("Flash (write)", func() {
		args := buildAvrdudeArgs(
			"-Uflash:w:%s:i",
			serialDropdown.Selected,
			progSelect.Selected,
			chipSelect.Selected,
			hexEntry.Text,
			confEntry.Text,
			forceChk.Checked,
			disableVerifyChk.Checked,
			disableEraseChk.Checked,
			eraseEEPROMChk.Checked,
			doNotWriteChk.Checked,
			true,
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox)
	})

	readBtn := widget.NewButton("Read", func() {
		args := buildAvrdudeArgs(
			"-Uflash:r:%s:i",
			serialDropdown.Selected,
			progSelect.Selected,
			chipSelect.Selected,
			hexEntry.Text,
			confEntry.Text,
			forceChk.Checked,
			disableVerifyChk.Checked,
			disableEraseChk.Checked,
			eraseEEPROMChk.Checked,
			doNotWriteChk.Checked,
			readToFileChk.Checked,
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox)
	})

	eraseBtn := widget.NewButton("Erase", func() {
		args := buildAvrdudeArgs(
			"",
			serialDropdown.Selected,
			progSelect.Selected,
			chipSelect.Selected,
			"",
			confEntry.Text,
			forceChk.Checked,
			disableVerifyChk.Checked,
			disableEraseChk.Checked,
			true,
			doNotWriteChk.Checked,
			false,
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox)
	})

	// Add a "Clear" button for the output
	clearBtn := widget.NewButton("Clear", func() {
		outputBox.SetText("") // wipe the output field
	})
	// clearBtn := widget.NewButton("Test", func() {
	// 	go func() {
	// 		appendOutput(outputBox, outputScroll, fmt.Sprintln("Running: test"))

	// 		cmd := exec.Command("python3", "progress_test.py")

	// 		// attach our GUI writer directly
	// 		cmd.Stdout = &guiWriter{outputBox, outputScroll}
	// 		cmd.Stderr = &guiWriter{outputBox, outputScroll}

	// 		go func() {
	// 			err := cmd.Run() // handles Start + Wait internally
	// 			if err != nil {
	// 				appendOutput(outputBox, outputScroll, fmt.Sprintf("avrdude finished with error: %v\n", err))
	// 			} else {
	// 				appendOutput(outputBox, outputScroll, "avrdude finished successfully\n")
	// 			}
	// 		}()
	// 	}()
	// })

	outputHeader := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("avrdude Output:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		clearBtn,
	)

	content := container.NewVBox(
		progChipRow,
		widget.NewLabelWithStyle("Serial port:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		serialDropdown,
		widget.NewLabelWithStyle("HEX file:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		hexRow,
		container.NewGridWithColumns(3, flashBtn, readBtn, eraseBtn),
		advancedAccordion,
		widget.NewSeparator(),
		outputHeader,
		outputBox,
	)

	// Add padding around content to avoid overlapping scrollbars (window vs output)
	padded := container.NewPadded(content)
	// Wrap the whole content in a VScroll so that when Advanced opens
	// and content grows, the window becomes scrollable instead of expanding.
	rootScroll := container.NewVScroll(padded)
	// set parent scroll of PassiveEntrys
	hexEntry.ParentScroll = rootScroll
	confEntry.ParentScroll = rootScroll
	baudEntry.ParentScroll = rootScroll
	w.SetContent(rootScroll)
	w.ShowAndRun()
}

// buildAvrdudeArgs composes arguments including advanced flags.
func buildAvrdudeArgs(writeTemplate, port, programmer, chip, hexfile, confOverride string,
	force, disableVerify, disableErase, eraseEEPROM, doNotWrite, useFile bool,
	verbosity, baud string) []string {

	confPath := confOverride
	if confPath == "" {
		confPath = filepath.Join("avrdude", "avrdude.conf")
	}

	var args []string
	args = append(args, "-C", confPath)

	// verbosity: 0 => none, 1..n => repeat -v that many times
	if verbosity != "" && verbosity != "0" {
		// convert verbosity string (0..4) into repeated -v
		switch verbosity {
		case "1":
			args = append(args, "-v")
		case "2":
			args = append(args, "-v", "-v")
		case "3":
			args = append(args, "-v", "-v", "-v")
		case "4":
			args = append(args, "-v", "-v", "-v", "-v")
		default:
			// ignore invalid
		}
	}

	// global flags from advanced options
	if force {
		args = append(args, "-F")
	}
	if disableVerify {
		args = append(args, "-V")
	}
	if disableErase {
		args = append(args, "-D")
	}
	if eraseEEPROM {
		args = append(args, "-e")
	}
	if doNotWrite {
		args = append(args, "-n")
	}

	if port != "" && port != "<no ports found>" {
		args = append(args, "-P", port)
	}
	if baud != "" {
		args = append(args, "-b", baud)
	}

	// programmer and chip
	if programmer != "" {
		args = append(args, "-c", programmer)
	}
	if chip != "" {
		args = append(args, "-p", chip)
	}

	// operation (flash read/write or erase) - writeTemplate already contains formatted tokens if needed
	if writeTemplate != "" {
		if hexfile == "" || !useFile {
			hexfile = "-"
		}
		args = append(args, fmt.Sprintf(writeTemplate, hexfile))
	}

	return args
}

// locateAvrdude finds avrdude in ./bin/<os>/avrdude or one of the alternative paths
func locateAvrdude() (string, error) {
	base := "./avrdude"
	bin := "bin"
	osname := runtime.GOOS
	arch := runtime.GOARCH
	candidates := []string{
		filepath.Join(base, fmt.Sprintf("%s_%s", osname, arch), bin, exeName()),
		filepath.Join(base, osname, bin, exeName()),
		filepath.Join(base, bin, exeName()),
		filepath.Join(base, exeName()),
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if osname != "windows" {
				// best-effort ensure exec bit
				_ = os.Chmod(p, 0755)
			}
			return p, nil
		}
	}
	return "", errors.New("avrdude executable not found in ./avrdude; please place avrdude under ./avrdude/<os>/bin")
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "avrdude.exe"
	}
	return "avrdude"
}

// runAvrdudeAndAttachOutput streams avrdude stdout/stderr into the output box and attempts to keep it scrolled to bottom.
type guiWriter struct {
	outputBox *ReadOnlyMultilineEntry
}

func (w *guiWriter) Write(p []byte) (int, error) {
	appendOutput(w.outputBox, string(p))
	return len(p), nil
}

func runAvrdudeAndAttachOutput(args []string, outputBox *ReadOnlyMultilineEntry) {
	avrdudePath, err := locateAvrdude()
	if err != nil {
		appendOutput(outputBox, fmt.Sprintf("ERROR: %v\n", err))
		return
	}

	appendOutput(outputBox, fmt.Sprintf("Running: %s %s\n", avrdudePath, strings.Join(args, " ")))

	cmd := exec.Command(avrdudePath, args...)

	// attach our GUI writer directly
	cmd.Stdout = &guiWriter{outputBox}
	cmd.Stderr = &guiWriter{outputBox}

	go func() {
		err := cmd.Run() // handles Start + Wait internally
		if err != nil {
			appendOutput(outputBox, fmt.Sprintf("avrdude finished with error: %v\n", err))
		} else {
			appendOutput(outputBox, "avrdude finished successfully\n")
		}
	}()
}

// appendOutput appends text to the output entry and scrolls the output Scroll container to bottom.
// It also handles carriage returns (\r) to overwrite the current line for progress bars
func appendOutput(outputBox *ReadOnlyMultilineEntry, text string) {
	curr := outputBox.Text
	for _, ch := range text {
		if ch == '\r' {
			// remove last line from curr
			lastNewline := strings.LastIndex(curr, "\n")
			if lastNewline >= 0 {
				curr = curr[:lastNewline+1] // keep everything up to and including the newline
			} else {
				curr = "" // no newline yet, clear whole buffer
			}
		} else {
			curr += string(ch)
		}
	}

	// Prevent runaway memory growth
	if len(curr) > 200000 {
		curr = curr[len(curr)-180000:]
	}

	// update UI on main thread
	outputBox.SetText(curr)
}
