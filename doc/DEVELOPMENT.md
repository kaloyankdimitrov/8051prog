# Development Documentation

**WIP: This documentation is still incomplete.**

If interested in contributing to the development of 8051prog or understanding more about the project, make sure to read this document.

## Golang&Fyne

The software is written in Go and call code is in the main.go file. The Fyne framework is used for the graphical interface.

## Avrdude

Avrdude version 6.3 is used and is package with the program for each operating system. Newer versions do **NOT** work with the AT89 boards. A custom configuration that includes the necessary definitions for this family of chips.The stk500v1 programmer type works with the ArduinoISP.

## Building

Building for each operating system is currently done natively on that operating system. Build scripts for each are in the main source folder. They produce a zip file for Windows and MacOS, and a .tar.gz file for linux.
Fyne cross-compilation could be used in the future; the MacOS SDK has to be obtained for it.
