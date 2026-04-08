//go:build windows

package main

import "os/exec"

func detachDaemonProcess(cmd *exec.Cmd) {}
