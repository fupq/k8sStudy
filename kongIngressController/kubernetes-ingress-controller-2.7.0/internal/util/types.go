/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
)

// Endpoint describes a kubernetes endpoint, same as a target in Kong.
type Endpoint struct {
	// Address IP address of the endpoint
	Address string `json:"address"`
	// Port number of the TCP port
	Port string `json:"port"`
}

// RawSSLCert represnts TLS cert and key in bytes.
type RawSSLCert struct {
	Cert []byte
	Key  []byte
}

type ConfigDumpMode int

const (
	ConfigDumpModeOff       ConfigDumpMode = iota
	ConfigDumpModeEnabled   ConfigDumpMode = iota
	ConfigDumpModeSensitive ConfigDumpMode = iota
)

func ParseConfigDumpMode(in string) (ConfigDumpMode, error) {
	switch in {
	case "enabled":
		return ConfigDumpModeEnabled, nil
	case "sensitive":
		return ConfigDumpModeSensitive, nil
	case "":
		return ConfigDumpModeOff, nil
	default:
		return ConfigDumpModeOff, fmt.Errorf("unrecognized config dump mode: %s", in)
	}
}
