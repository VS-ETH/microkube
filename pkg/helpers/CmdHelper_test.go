/*
 * Copyright 2018 The microkube authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package helpers

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestInvalidInvocation tests the invocation of a non-existent program
func TestInvalidInvocation(t *testing.T) {
	exitWaiter := make(chan bool)
	exitHandler := func(rc bool, error *exec.ExitError) {
		exitWaiter <- rc
	}
	handler := NewCmdHandler("/bin/FooBarBazBash", []string{
		"-c",
		"echo test",
	}, exitHandler, nil, nil)
	err := handler.Start()
	if err == nil {
		t.Error("Invalid command executed?")
		return
	}
}

// TestEchoInvocation tests running echo
func TestEchoInvocation(t *testing.T) {
	exitWaiter := make(chan bool)
	exitHandler := func(rc bool, error *exec.ExitError) {
		exitWaiter <- rc
	}
	handler := NewCmdHandler("/bin/bash", []string{
		"-c",
		"echo test",
	}, exitHandler, nil, nil)
	err := handler.Start()
	if err != nil {
		t.Error("Coudln't start program")
		return
	}
	rc := <-exitWaiter
	if !rc {
		t.Error("Couldn't execute echo!")
	}
}

// TestEcho tests running echo and comparing it's output
func TestEcho(t *testing.T) {
	exitWaiter := make(chan bool)
	exitStdout := make(chan string, 10)
	exitHandler := func(rc bool, error *exec.ExitError) {
		exitWaiter <- rc
	}
	stdoutHandler := func(value []byte) {
		exitStdout <- string(value)
	}
	handler := NewCmdHandler("/bin/bash", []string{
		"-c",
		"echo test",
	}, exitHandler, stdoutHandler, stdoutHandler)
	err := handler.Start()
	if err != nil {
		t.Fatalf("Coudln't start program")
		return
	}
	ctx, cfunc := context.WithTimeout(context.Background(), 2*time.Second)
	defer cfunc()
	exitChecked, stdoutChecked := false, false
	stdout := ""
	for {
		timeout := false
		select {
		case str := <-exitStdout:
			stdout = stdout + strings.Trim(str, " \t\r\n") + " "
			stdoutChecked = true
		case <-ctx.Done():
			timeout = true
		}
		if timeout {
			break
		}
	}
	ctx, cfunc = context.WithTimeout(context.Background(), 2*time.Second)
	defer cfunc()
	select {
	case <-exitWaiter:
		exitChecked = true
	case <-ctx.Done():
	}
	if !strings.Contains(stdout, "test") {
		t.Fatal("Unexpected stdout: '", stdout, "'")
	}
	if !stdoutChecked {
		t.Fatalf("Test timeouted, exitChecked: %t, stdoutChecked: %t", exitChecked, stdoutChecked)
	}
}

// TestAllBinariesPresent tries to find all binaries required during tests
func TestAllBinariesPresent(t *testing.T) {
	binaries := []string{
		"etcd",
		"hyperkube",
	}
	for _, item := range binaries {
		path, err := FindBinary(item, "", "")
		if err != nil {
			t.Fatal("Didn't find " + item)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal("Coudln't stat " + item)
		}
		if !info.Mode().IsRegular() {
			t.Fatal(item + "isn't a regular file")
		}
	}
}

// TestErrorReturn tests running a program with RC != 0
func TestErrorReturn(t *testing.T) {
	exitWaiter := make(chan bool)
	exitHandler := func(rc bool, errorCode *exec.ExitError) {
		if errorCode == nil {
			t.Fatalf("Expected error missing")
		}
		exitWaiter <- rc
	}
	handler := NewCmdHandler("/bin/bash", []string{
		"-c",
		"exit -1",
	}, exitHandler, nil, nil)
	err := handler.Start()
	if err != nil {
		t.Error("Coudln't start program")
		return
	}
	rc := <-exitWaiter
	if rc {
		t.Error("Unexpectedly successful return?")
	}
}

// TestProcessKill tests whether killing the process works
func TestProcessKill(t *testing.T) {
	exitWaiter := make(chan bool)
	exitHandler := func(rc bool, errorCode *exec.ExitError) {
		exitWaiter <- rc
	}
	handler := NewCmdHandler("/bin/bash", []string{
		"-c",
		"sleep 120",
	}, exitHandler, nil, nil)
	err := handler.Start()
	if err != nil {
		t.Error("Coudln't start program")
		return
	}
	// Wait two seconds which should be enough to start. This isn't exactly the best solution :/
	time.Sleep(2 * time.Second)
	// Kill the process
	handler.Stop()

	rc := <-exitWaiter
	if rc {
		t.Error("Unexpectedly successful return?")
	}
}
