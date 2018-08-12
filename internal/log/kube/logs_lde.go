
/*
 This file was autogenerated via
 ----------------------------------------------------
 ldetool generate --package kube --go-string logs.lde
 ----------------------------------------------------
 do not touch it with bare hands!
*/

package kube

import (
	"strings"
)

var bslashN = "\n"
var colonSpace = ": "
var lsbrckRestfulRsbrckSpace = "[restful] "
var lsbrckRestfulSlashSwaggerRsbrckSpace = "[restful/swagger] "
var rsbrckSpace = "] "
var space = " "

// KubeLogLine ...
type KubeLogLine struct {
	rest             string
	SeverityID       string
	Time             string
	UnknownAttribute string
	Location         string
	Message          string
}

// Extract ...
func (p *KubeLogLine) Extract(line string) (bool, error) {
	p.rest = line
	var pos int

	// Take until " " as SeverityID(string)
	pos = strings.Index(p.rest, space)
	if pos >= 0 {
		p.SeverityID = p.rest[:pos]
		p.rest = p.rest[pos+len(space):]
	} else {
		return false, nil
	}

	// Take until " " as Time(string)
	pos = strings.Index(p.rest, space)
	if pos >= 0 {
		p.Time = p.rest[:pos]
		p.rest = p.rest[pos+len(space):]
	} else {
		return false, nil
	}

	// Take until " " as UnknownAttribute(string)
	pos = strings.Index(p.rest, space)
	if pos >= 0 {
		p.UnknownAttribute = p.rest[:pos]
		p.rest = p.rest[pos+len(space):]
	} else {
		return false, nil
	}

	// Take until "] " as Location(string)
	pos = strings.Index(p.rest, rsbrckSpace)
	if pos >= 0 {
		p.Location = p.rest[:pos]
		p.rest = p.rest[pos+len(rsbrckSpace):]
	} else {
		return false, nil
	}

	// Take until "\n" as Message(string)
	pos = strings.Index(p.rest, bslashN)
	if pos >= 0 {
		p.Message = p.rest[:pos]
		p.rest = p.rest[pos+len(bslashN):]
	} else {
		return false, nil
	}

	return true, nil
}

// KubeLogLineRestful ...
type KubeLogLineRestful struct {
	rest     string
	Date     string
	Time     string
	Location string
	Message  string
}

// Extract ...
func (p *KubeLogLineRestful) Extract(line string) (bool, error) {
	p.rest = line
	var pos int

	// Checks if the rest starts with `"[restful] "` and pass it
	if strings.HasPrefix(p.rest, lsbrckRestfulRsbrckSpace) {
		p.rest = p.rest[len(lsbrckRestfulRsbrckSpace):]
	} else {
		return false, nil
	}

	// Take until " " as Date(string)
	pos = strings.Index(p.rest, space)
	if pos >= 0 {
		p.Date = p.rest[:pos]
		p.rest = p.rest[pos+len(space):]
	} else {
		return false, nil
	}

	// Take until " " as Time(string)
	pos = strings.Index(p.rest, space)
	if pos >= 0 {
		p.Time = p.rest[:pos]
		p.rest = p.rest[pos+len(space):]
	} else {
		return false, nil
	}

	// Take until ": " as Location(string)
	pos = strings.Index(p.rest, colonSpace)
	if pos >= 0 {
		p.Location = p.rest[:pos]
		p.rest = p.rest[pos+len(colonSpace):]
	} else {
		return false, nil
	}

	// Checks if the rest starts with `"[restful/swagger] "` and pass it
	if strings.HasPrefix(p.rest, lsbrckRestfulSlashSwaggerRsbrckSpace) {
		p.rest = p.rest[len(lsbrckRestfulSlashSwaggerRsbrckSpace):]
	} else {
		return false, nil
	}

	// Take until "\n" as Message(string)
	pos = strings.Index(p.rest, bslashN)
	if pos >= 0 {
		p.Message = p.rest[:pos]
		p.rest = p.rest[pos+len(bslashN):]
	} else {
		return false, nil
	}

	return true, nil
}
