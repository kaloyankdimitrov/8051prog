// main.go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

func (p *PassiveEntry) Scrolled(ev *fyne.ScrollEvent) {
	if p.ParentScroll != nil {
		p.ParentScroll.Scrolled(ev)
	}
}

func main() {
	myApp := app.NewWithID("8051prog")
	w := myApp.NewWindow("8051 Programmer")
	w.Resize(fyne.NewSize(900, 600))
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
		serialDropdown.Options = ports
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
	chooseHexBtn.Importance = widget.LowImportance // make it small/flat
	hexRow := container.NewBorder(nil, nil, nil, chooseHexBtn, hexEntry)

	// Output area: read-only multiline entry with both-axis scrolling and no wrap
	outputBox := widget.NewMultiLineEntry()
	outputScroll := container.NewScroll(outputBox)
	outputScroll.SetMinSize(fyne.NewSize(800, 320))

	// Advanced options widgets
	forceChk := widget.NewCheck("Force (-F)", nil)
	disableVerifyChk := widget.NewCheck("Disable verify (-V)", nil)
	disableEraseChk := widget.NewCheck("Disable flash erase (-D)", nil)
	eraseEEPROMChk := widget.NewCheck("Erase flash and EEPROM (-e)", nil)
	doNotWriteChk := widget.NewCheck("Do not write (-n)", nil)

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
	chooseConfBtn.Importance = widget.LowImportance
	confRow := container.NewBorder(nil, nil, nil, chooseConfBtn, confEntry)

	advancedGrid := container.NewVBox(
		container.NewGridWithColumns(2, forceChk, eraseEEPROMChk),
		container.NewGridWithColumns(2, disableVerifyChk, doNotWriteChk),
		container.NewGridWithColumns(2, disableEraseChk, widget.NewLabel("")),
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
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox, outputScroll, w)
	})

	readBtn := widget.NewButton("Read (flash -> file)", func() {
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
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox, outputScroll, w)
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
			verbosity.Selected,
			baudEntry.Text,
		)
		runAvrdudeAndAttachOutput(args, outputBox, outputScroll, w)
	})

	content := container.NewVBox(
		progChipRow,
		widget.NewLabelWithStyle("Serial port:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		serialDropdown,
		widget.NewLabelWithStyle("HEX file:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		hexRow,
		container.NewGridWithColumns(3, flashBtn, readBtn, eraseBtn),
		advancedAccordion,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("avrdude Output:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		outputScroll,
	)

	// Add padding around content to avoid overlapping scrollbars (window vs output)
	padded := container.NewPadded(content)
	// Wrap the whole content in a VScroll so that when Advanced opens
	// and content grows, the window becomes scrollable instead of expanding.
	rootScroll := container.NewVScroll(padded)
	w.SetContent(rootScroll)
	w.ShowAndRun()
}

// buildAvrdudeArgs composes arguments including advanced flags.
func buildAvrdudeArgs(writeTemplate, port, programmer, chip, hexfile, confOverride string,
	force, disableVerify, disableErase, eraseEEPROM, doNotWrite bool,
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
		if hexfile == "" {
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
func runAvrdudeAndAttachOutput(args []string, outputBox *widget.Entry, outputScroll *container.Scroll, win fyne.Window) {
	avrdudePath, err := locateAvrdude()
	if err != nil {
		appendOutput(outputBox, outputScroll, fmt.Sprintf("ERROR: %v\n", err))
		return
	}

	appendOutput(outputBox, outputScroll, fmt.Sprintf("Running: %s %s\n", avrdudePath, strings.Join(args, " ")))

	cmd := exec.Command(avrdudePath, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		appendOutput(outputBox, outputScroll, fmt.Sprintf("failed to start avrdude: %v\n", err))
		return
	}

	go scanPipeToOutput(stdout, outputBox, outputScroll)
	go scanPipeToOutput(stderr, outputBox, outputScroll)

	go func() {
		err := cmd.Wait()
		if err != nil {
			appendOutput(outputBox, outputScroll, fmt.Sprintf("avrdude finished with error: %v\n", err))
		} else {
			appendOutput(outputBox, outputScroll, "avrdude finished successfully\n")
		}
	}()
}

func scanPipeToOutput(pipe io.ReadCloser, outputBox *widget.Entry, outputScroll *container.Scroll) {
	s := bufio.NewScanner(pipe)
	for s.Scan() {
		line := s.Text()
		appendOutput(outputBox, outputScroll, line+"\n")
	}
}

// appendOutput appends text to the output entry and scrolls the output Scroll container to bottom
func appendOutput(outputBox *widget.Entry, outputScroll *container.Scroll, text string) {
	curr := outputBox.Text
	curr += text
	if len(curr) > 200000 {
		curr = curr[len(curr)-180000:]
	}
	// update UI on main thread
	fyne.CurrentApp().SendNotification(&fyne.Notification{Title: "", Content: ""}) // no-op to ensure UI loop; harmless
	outputBox.Text = curr
	outputBox.Refresh()
	// Only scroll if content exceeds the viewport height, so short output stays fully visible.
	if outputScroll != nil {
		contentH := outputScroll.Content.Size().Height
		viewH := outputScroll.Size().Height
		if contentH > viewH {
			outputScroll.ScrollToBottom()
		}
	}
}
